package migrator

import (
	"context"
	"fmt"
	"net/http"

	flag "github.com/spf13/pflag"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/shutdown"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hornet/v2/pkg/common"
	validator "github.com/iotaledger/hornet/v2/pkg/model/migrator"
	"github.com/iotaledger/inx-coordinator/pkg/daemon"
	"github.com/iotaledger/inx-coordinator/pkg/migrator"
	legacyapi "github.com/iotaledger/iota.go/api"
	iotago "github.com/iotaledger/iota.go/v3"
)

const (
	// CfgMigratorBootstrap configures whether the migration process is bootstrapped.
	CfgMigratorBootstrap = "migratorBootstrap"
	// CfgMigratorStartIndex configures the index of the first milestone to migrate.
	CfgMigratorStartIndex = "migratorStartIndex"
)

func init() {
	Component = &app.Component{
		Name:      "Migrator",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		IsEnabled: func(_ *dig.Container) bool { return ParamsMigrator.Enabled },
		Provide:   provide,
		Configure: configure,
		Run:       run,
	}
}

var (
	Component *app.Component
	deps      dependencies

	bootstrap  = flag.Bool(CfgMigratorBootstrap, false, "bootstrap the migration process")
	startIndex = flag.Uint32(CfgMigratorStartIndex, 1, "index of the first milestone to migrate")
)

type dependencies struct {
	dig.In
	MigratorService *migrator.Service
	ShutdownHandler *shutdown.ShutdownHandler
}

// provide provides the MigratorService as a singleton.
func provide(c *dig.Container) error {

	if err := c.Provide(func() *validator.Validator {
		legacyAPI, err := legacyapi.ComposeAPI(legacyapi.HTTPClientSettings{
			URI:    ParamsReceipts.Validator.API.Address,
			Client: &http.Client{Timeout: ParamsReceipts.Validator.API.Timeout},
		})
		if err != nil {
			Component.LogErrorfAndExit("failed to initialize API: %s", err)
		}

		return validator.NewValidator(
			legacyAPI,
			ParamsReceipts.Validator.Coordinator.Address,
			ParamsReceipts.Validator.Coordinator.MerkleTreeDepth,
		)
	}); err != nil {
		return err
	}

	type serviceDeps struct {
		dig.In
		Validator *validator.Validator
	}

	if err := c.Provide(func(deps serviceDeps) *migrator.Service {

		maxReceiptEntries := ParamsMigrator.ReceiptMaxEntries
		switch {
		case maxReceiptEntries > iotago.MaxMigratedFundsEntryCount:
			Component.LogErrorfAndExit("%s (set to %d) can be max %d", Component.App().Config().GetParameterPath(&(ParamsMigrator.ReceiptMaxEntries)), maxReceiptEntries, iotago.MaxMigratedFundsEntryCount)
		case maxReceiptEntries <= 0:
			Component.LogErrorfAndExit("%s must be greather than 0", Component.App().Config().GetParameterPath(&(ParamsMigrator.ReceiptMaxEntries)))
		}

		return migrator.NewService(
			deps.Validator,
			ParamsMigrator.StateFilePath,
			ParamsMigrator.ReceiptMaxEntries,
		)
	}); err != nil {
		return err
	}

	return nil
}

func configure() error {

	var msIndex *iotago.MilestoneIndex
	if *bootstrap {
		msIndex = startIndex
	}

	if err := deps.MigratorService.InitState(msIndex); err != nil {
		Component.LogFatalfAndExit("failed to initialize migrator: %s", err)
	}

	return nil
}

func run() error {

	if err := Component.App().Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		Component.LogInfof("Starting %s ... done", Component.Name)
		deps.MigratorService.Start(ctx, func(err error) bool {

			if err := common.IsCriticalError(err); err != nil {
				deps.ShutdownHandler.SelfShutdown(fmt.Sprintf("migrator plugin hit a critical error: %s", err), true)

				return false
			}

			if err := common.IsSoftError(err); err != nil {
				deps.MigratorService.Events.SoftError.Trigger(err)
			}

			// lets just log the err and halt querying for a configured period
			Component.LogWarn(err)

			return timeutil.Sleep(ctx, ParamsMigrator.QueryCooldownPeriod)
		})
		Component.LogInfof("Stopping %s ... done", Component.Name)
	}, daemon.PriorityStopMigrator); err != nil {
		return err
	}

	return nil
}
