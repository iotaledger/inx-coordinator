package coordinator

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/shutdown"
	"github.com/iotaledger/hive.go/crypto"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/valuenotifier"
	"github.com/iotaledger/hornet/v2/pkg/common"
	"github.com/iotaledger/inx-app/pkg/nodebridge"
	"github.com/iotaledger/inx-coordinator/pkg/coordinator"
	"github.com/iotaledger/inx-coordinator/pkg/daemon"
	"github.com/iotaledger/inx-coordinator/pkg/migrator"
	"github.com/iotaledger/inx-coordinator/pkg/mselection"
	"github.com/iotaledger/inx-coordinator/pkg/todo"
	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v3"
	"github.com/iotaledger/iota.go/v3/keymanager"
)

const (
	// CfgCoordinatorBootstrap defines whether to bootstrap the network.
	CfgCoordinatorBootstrap = "cooBootstrap"
	// CfgCoordinatorStartIndex defines the index of the first milestone at bootstrap.
	CfgCoordinatorStartIndex = "cooStartIndex"
	// MilestoneMaxAdditionalTipsLimit defines the maximum limit of additional tips that fit into a milestone (besides the last milestone and checkpoint hash).
	MilestoneMaxAdditionalTipsLimit = 6
)

var (
	ErrDatabaseTainted = errors.New("database is tainted. delete the coordinator database and start again with a snapshot")
)

func init() {
	Component = &app.Component{
		Name:      "Coordinator",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		Provide:   provide,
		Configure: configure,
		Run:       run,
	}
}

var (
	Component *app.Component
	deps      dependencies

	bootstrap  = flag.Bool(CfgCoordinatorBootstrap, false, "bootstrap the network")
	startIndex = flag.Uint32(CfgCoordinatorStartIndex, 0, "index of the first milestone at bootstrap")

	nextCheckpointSignal chan struct{}
	nextMilestoneSignal  chan struct{}

	heaviestSelectorLock syncutils.RWMutex

	lastCheckpointIndex   int
	lastCheckpointBlockID iotago.BlockID
	lastMilestoneBlockID  iotago.BlockID
)

type dependencies struct {
	dig.In
	Coordinator      *coordinator.Coordinator
	Selector         *mselection.HeaviestSelector
	NodeBridge       *nodebridge.NodeBridge
	ShutdownHandler  *shutdown.ShutdownHandler
	TangleListener   *nodebridge.TangleListener
	TreasuryListener *TreasuryListener `optional:"true"`
}

func provide(c *dig.Container) error {

	if err := c.Provide(func() *mselection.HeaviestSelector {
		// use the heaviest branch tip selection for the milestones
		return mselection.New(
			ParamsCoordinator.TipSel.MinHeaviestBranchUnreferencedBlocksThreshold,
			ParamsCoordinator.TipSel.MaxHeaviestBranchTipsPerCheckpoint,
			ParamsCoordinator.TipSel.RandomTipsPerCheckpoint,
			ParamsCoordinator.TipSel.HeaviestBranchSelectionTimeout,
		)
	}); err != nil {
		return err
	}

	type coordinatorDeps struct {
		dig.In
		MigratorService *migrator.Service `optional:"true"`
		NodeBridge      *nodebridge.NodeBridge
	}

	type coordinatorDepsOut struct {
		dig.Out
		Coordinator      *coordinator.Coordinator
		TangleListener   *nodebridge.TangleListener
		TreasuryListener *TreasuryListener `optional:"true"`
	}

	if err := c.Provide(func(deps coordinatorDeps) coordinatorDepsOut {

		var treasuryListener *TreasuryListener
		if deps.MigratorService != nil {
			treasuryListener = NewTreasuryListener(Component.Logger(), deps.NodeBridge)
		}

		initCoordinator := func() (*coordinator.Coordinator, error) {

			keyManager := keymanager.New()
			for _, keyRange := range deps.NodeBridge.NodeConfig.GetMilestoneKeyRanges() {
				keyManager.AddKeyRange(keyRange.GetPublicKey(), keyRange.GetStartIndex(), keyRange.GetEndIndex())
			}

			signingProvider, err := initSigningProvider(
				ParamsCoordinator.Signing.Provider,
				ParamsCoordinator.Signing.RemoteAddress,
				keyManager,
				int(deps.NodeBridge.NodeConfig.GetMilestonePublicKeyCount()),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize signing provider: %w", err)
			}

			if ParamsCoordinator.Quorum.Enabled {
				Component.LogInfo("running coordinator with quorum enabled")
			}

			if deps.MigratorService == nil {
				Component.LogInfo("running coordinator without migration enabled")
			}

			coo, err := coordinator.New(
				ComputeMerkleTreeHash,
				deps.NodeBridge.IsNodeSynced,
				deps.NodeBridge.ProtocolParameters,
				signingProvider,
				deps.MigratorService,
				treasuryListener.LatestTreasuryOutput,
				sendBlock,
				coordinator.WithLogger(Component.Logger()),
				coordinator.WithStateFilePath(ParamsCoordinator.StateFilePath),
				coordinator.WithMilestoneInterval(ParamsCoordinator.Interval),
				coordinator.WithMilestoneTimeout(ParamsCoordinator.MilestoneTimeout),
				coordinator.WithQuorum(ParamsCoordinator.Quorum.Enabled, ParamsCoordinator.Quorum.Groups, ParamsCoordinator.Quorum.Timeout),
				coordinator.WithSigningRetryAmount(ParamsCoordinator.Signing.RetryAmount),
				coordinator.WithSigningRetryTimeout(ParamsCoordinator.Signing.RetryTimeout),
				coordinator.WithBlockBackups(ParamsCoordinator.BlockBackups.Enabled, ParamsCoordinator.BlockBackups.FolderPath),
				coordinator.WithDebugFakeMilestoneTimestamps(ParamsCoordinator.DebugFakeMilestoneTimestamps),
			)
			if err != nil {
				return nil, err
			}

			latestMilestone := &coordinator.LatestMilestoneInfo{
				Index:       0,
				Timestamp:   0,
				MilestoneID: iotago.MilestoneID{},
			}

			ms, err := deps.NodeBridge.LatestMilestone()
			if err != nil {
				return nil, err
			}
			if ms != nil {
				latestMilestone = &coordinator.LatestMilestoneInfo{
					Index:       ms.Milestone.Index,
					Timestamp:   ms.Milestone.Timestamp,
					MilestoneID: ms.MilestoneID,
				}
			} else {
				// in case we start with a global snapshot on L1, the last milestone might be unknown
				latestMilestone.Index = deps.NodeBridge.LatestMilestoneIndex()
			}

			if err := coo.InitState(*bootstrap, *startIndex, latestMilestone); err != nil {
				return nil, err
			}

			// don't issue milestones or checkpoints in case the node is running hot
			coo.AddBackPressureFunc(todo.IsNodeTooLoaded)

			return coo, nil
		}

		coo, err := initCoordinator()
		if err != nil {
			Component.LogErrorAndExit(err)
		}

		return coordinatorDepsOut{
			Coordinator:      coo,
			TangleListener:   nodebridge.NewTangleListener(deps.NodeBridge),
			TreasuryListener: treasuryListener,
		}
	}); err != nil {
		return err
	}

	return nil
}

func configure() error {

	databasesTainted, err := todo.AreDatabasesTainted()
	if err != nil {
		Component.LogErrorAndExit(err)
	}

	if databasesTainted {
		Component.LogErrorAndExit(ErrDatabaseTainted)
	}

	nextCheckpointSignal = make(chan struct{})

	// must be a buffered channel, otherwise signal gets
	// lost if checkpoint is generated at the same time
	nextMilestoneSignal = make(chan struct{}, 1)

	return nil
}

// handleError checks for critical errors and returns true if the node should shutdown.
func handleError(err error) bool {
	if err == nil {
		return false
	}

	if err := common.IsCriticalError(err); err != nil {
		deps.ShutdownHandler.SelfShutdown(fmt.Sprintf("coordinator plugin hit a critical error: %s", err), true)

		return true
	}

	if err := common.IsSoftError(err); err != nil {
		Component.LogWarn(err)
		deps.Coordinator.Events.SoftError.Trigger(err)

		return false
	}

	// this should not happen! errors should be defined as a soft or critical error explicitly
	Component.LogErrorfAndExit("coordinator plugin hit an unknown error type: %s", err)

	return true
}

func run() error {

	// create a background worker that signals to issue new milestones
	if err := Component.Daemon().BackgroundWorker("Coordinator[MilestoneTicker]", func(ctx context.Context) {
		Component.LogInfo("Start MilestoneTicker")
		ticker := timeutil.NewTicker(func() {
			// issue next milestone
			select {
			case nextMilestoneSignal <- struct{}{}:
			default:
				// do not block if already another signal is waiting
			}
		}, deps.Coordinator.Interval(), ctx)
		ticker.WaitForGracefulShutdown()
	}, daemon.PriorityStopCoordinatorMilestoneTicker); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	if err := Component.Daemon().BackgroundWorker("Coordinator[TangleListener]", func(ctx context.Context) {
		Component.LogInfo("Start TangleListener")
		deps.TangleListener.Run(ctx)
		Component.LogInfo("Stopped TangleListener")
	}, daemon.PriorityStopTangleListener); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	if deps.TreasuryListener != nil {
		if err := Component.Daemon().BackgroundWorker("Coordinator[TreasuryListener]", func(ctx context.Context) {
			Component.LogInfo("Start TreasuryListener")
			deps.TreasuryListener.Run(ctx)
			Component.LogInfo("Stopped TreasuryListener")
		}, daemon.PriorityStopTreasuryListener); err != nil {
			Component.LogPanicf("failed to start worker: %s", err)
		}
	}

	// create a background worker that issues milestones
	if err := Component.Daemon().BackgroundWorker("Coordinator", func(ctx context.Context) {
		unhookEvents := attachEvents()

		// bootstrap the network if not done yet
		//nolint:contextcheck // false positive
		milestoneBlockID, err := deps.Coordinator.Bootstrap()
		if handleError(err) {
			// critical error => stop worker
			unhookEvents()

			return
		}

		// init the last milestone block ID
		lastMilestoneBlockID = milestoneBlockID

		// init the checkpoints
		lastCheckpointBlockID = milestoneBlockID
		lastCheckpointIndex = 0

	coordinatorLoop:
		for {
			select {
			case <-nextCheckpointSignal:
				// check the thresholds again, because a new milestone could have been issued in the meantime
				if trackedBlocksCount := deps.Selector.TrackedBlocksCount(); trackedBlocksCount < ParamsCoordinator.Checkpoints.MaxTrackedBlocks {
					continue
				}

				func() {
					// this lock is necessary, otherwise a checkpoint could be issued
					// while a milestone gets confirmed. In that case the checkpoint could
					// contain blocks that are already below max depth.
					heaviestSelectorLock.RLock()
					defer heaviestSelectorLock.RUnlock()

					tips, err := deps.Selector.SelectTips(0)
					if err != nil {
						// issuing checkpoint failed => not critical
						if !errors.Is(err, mselection.ErrNoTipsAvailable) {
							Component.LogWarn(err)
						}

						return
					}

					// issue a checkpoint
					checkpointBlockID, err := deps.Coordinator.IssueCheckpoint(lastCheckpointIndex, lastCheckpointBlockID, tips)
					if err != nil {
						// issuing checkpoint failed => not critical
						Component.LogWarn(err)

						return
					}
					lastCheckpointIndex++
					lastCheckpointBlockID = checkpointBlockID
				}()

			case <-nextMilestoneSignal:
				var milestoneTips iotago.BlockIDs

				// skip signal to prevent milestone issuance with same timestamp
				if !deps.Coordinator.DebugFakeMilestoneTimestamps() && deps.Coordinator.State().LatestMilestoneTime.Unix() == time.Now().Unix() {
					Component.LogWarn("skipping milestone signal due to too fast ticker")

					continue
				}

				// issue a new checkpoint right in front of the milestone
				checkpointTips, err := deps.Selector.SelectTips(1)
				if err != nil {
					// issuing checkpoint failed => not critical
					if !errors.Is(err, mselection.ErrNoTipsAvailable) {
						Component.LogWarn(err)
					}
				} else {
					if len(checkpointTips) > MilestoneMaxAdditionalTipsLimit {
						// issue a checkpoint with all the tips that wouldn't fit into the milestone (more than MilestoneMaxAdditionalTipsLimit)
						checkpointBlockID, err := deps.Coordinator.IssueCheckpoint(lastCheckpointIndex, lastCheckpointBlockID, checkpointTips[MilestoneMaxAdditionalTipsLimit:])
						if err != nil {
							// issuing checkpoint failed => not critical
							Component.LogWarn(err)
						} else {
							// use the new checkpoint block ID
							lastCheckpointBlockID = checkpointBlockID
						}

						// use the other tips for the milestone
						milestoneTips = checkpointTips[:MilestoneMaxAdditionalTipsLimit]
					} else {
						// do not issue a checkpoint and use the tips for the milestone instead since they fit into the milestone directly
						milestoneTips = checkpointTips
					}
				}

				milestoneTips = append(milestoneTips, iotago.BlockIDs{lastMilestoneBlockID, lastCheckpointBlockID}...)

				//nolint:contextcheck // false positive
				milestoneBlockID, err := deps.Coordinator.IssueMilestone(milestoneTips)
				if handleError(err) {
					// critical error => quit loop
					break coordinatorLoop
				}
				if err != nil {
					// non-critical errors
					if errors.Is(err, common.ErrNodeNotSynced) {
						// Coordinator is not synchronized, trigger the solidifier manually
						todo.TriggerSolidifier()
					}

					// reset the checkpoints
					lastCheckpointBlockID = lastMilestoneBlockID
					lastCheckpointIndex = 0

					continue
				}

				// remember the last milestone block ID
				lastMilestoneBlockID = milestoneBlockID

				// reset the checkpoints
				lastCheckpointBlockID = milestoneBlockID
				lastCheckpointIndex = 0

			case <-ctx.Done():
				break coordinatorLoop
			}
		}

		deps.Coordinator.StopMilestoneTimeoutTicker()
		unhookEvents()
	}, daemon.PriorityStopCoordinator); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}

// loadEd25519PrivateKeysFromEnvironment loads ed25519 private keys from the given environment variable.
func loadEd25519PrivateKeysFromEnvironment(name string) ([]ed25519.PrivateKey, error) {

	keys, exists := os.LookupEnv(name)
	if !exists {
		return nil, fmt.Errorf("environment variable '%s' not set", name)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("environment variable '%s' not set", name)
	}

	privateKeysSplitted := strings.Split(keys, ",")
	privateKeys := make([]ed25519.PrivateKey, len(privateKeysSplitted))
	for i, key := range privateKeysSplitted {
		privateKey, err := crypto.ParseEd25519PrivateKeyFromString(key)
		if err != nil {
			return nil, fmt.Errorf("environment variable '%s' contains an invalid private key '%s'", name, key)

		}
		privateKeys[i] = privateKey
	}

	return privateKeys, nil
}

func initSigningProvider(signingProviderType string, remoteEndpoint string, keyManager *keymanager.KeyManager, milestonePublicKeyCount int) (coordinator.MilestoneSignerProvider, error) {

	switch signingProviderType {
	case "local":
		privateKeys, err := loadEd25519PrivateKeysFromEnvironment("COO_PRV_KEYS")
		if err != nil {
			return nil, err
		}

		if len(privateKeys) == 0 {
			return nil, errors.New("no private keys given")
		}

		for _, privateKey := range privateKeys {
			if len(privateKey) != ed25519.PrivateKeySize {
				return nil, errors.New("wrong private key length")
			}
		}

		return coordinator.NewInMemoryEd25519MilestoneSignerProvider(privateKeys, keyManager, milestonePublicKeyCount), nil

	case "remote":
		if remoteEndpoint == "" {
			return nil, errors.New("no address given for remote signing provider")
		}

		return coordinator.NewInsecureRemoteEd25519MilestoneSignerProvider(remoteEndpoint, keyManager, milestonePublicKeyCount), nil

	default:
		return nil, fmt.Errorf("unknown milestone signing provider: %s", signingProviderType)
	}
}

func sendBlock(block *iotago.Block, msIndex ...iotago.MilestoneIndex) (iotago.BlockID, error) {

	var milestoneConfirmedListener *valuenotifier.Listener
	if len(msIndex) > 0 {
		milestoneConfirmedListener = deps.TangleListener.RegisterMilestoneConfirmedEvent(msIndex[0])
		defer milestoneConfirmedListener.Deregister()
	}

	var blockID iotago.BlockID
	blockID, err := deps.NodeBridge.SubmitBlock(Component.Daemon().ContextStopped(), block)
	if err != nil {
		return iotago.EmptyBlockID(), err
	}

	blockSolidListener, err := deps.TangleListener.RegisterBlockSolidEvent(context.Background(), blockID)
	if err != nil {
		return iotago.EmptyBlockID(), err
	}
	defer blockSolidListener.Deregister()

	// wait until the block is solid
	if err := blockSolidListener.Wait(context.Background()); err != nil {
		return iotago.EmptyBlockID(), err
	}

	if len(msIndex) > 0 {
		// if it was a milestone, also wait until the milestone was confirmed
		if err := milestoneConfirmedListener.Wait(context.Background()); err != nil {
			return iotago.EmptyBlockID(), err
		}
	}

	return blockID, nil
}

func attachEvents() func() {

	// pass all new solid blocks to the selector
	onBlockSolid := func(metadata *inx.BlockMetadata) {

		if !deps.NodeBridge.IsNodeSynced() {
			// ignore tips if the node is not synced,
			// otherwise we may add blocks that seem to be fine,
			// but are below max depth in reality
			return
		}

		if metadata.GetShouldReattach() {
			// ignore tips that are below max depth
			return
		}

		// add tips to the heaviest branch selector
		if trackedBlocksCount := deps.Selector.OnNewSolidBlock(metadata); trackedBlocksCount >= ParamsCoordinator.Checkpoints.MaxTrackedBlocks {
			Component.LogDebugf("Coordinator Tipselector: trackedBlocksCount: %d", trackedBlocksCount)

			// issue next checkpoint
			select {
			case nextCheckpointSignal <- struct{}{}:
			default:
				// do not block if already another signal is waiting
			}
		}
	}

	onLatestMilestoneChanged := func(_ *nodebridge.Milestone) {
		// reset the milestone timeout ticker
		deps.Coordinator.ResetMilestoneTimeoutTicker()

		// re-enable the tipselector in case it was disabled
		deps.Selector.Continue()
	}

	onConfirmedMilestoneChanged := func(_ *nodebridge.Milestone) {
		heaviestSelectorLock.Lock()
		defer heaviestSelectorLock.Unlock()

		// the selector needs to be reset after the milestone was confirmed, otherwise
		// it could contain tips that are already below max depth.
		deps.Selector.Reset()

		// the checkpoint also needs to be reset, otherwise
		// a checkpoint could have been issued in the meantime,
		// which could contain blocks that are already below max depth.
		lastCheckpointBlockID = lastMilestoneBlockID
		lastCheckpointIndex = 0
	}

	onMilestoneTimeout := func() {
		deps.Selector.Stop()
	}

	onIssuedCheckpoint := func(checkpointIndex int, tipIndex int, tipsTotal int, blockID iotago.BlockID) {
		Component.LogInfof("checkpoint (%d) block issued (%d/%d): %v", checkpointIndex+1, tipIndex+1, tipsTotal, blockID.ToHex())
	}

	onIssuedMilestone := func(index iotago.MilestoneIndex, milestoneID iotago.MilestoneID, blockID iotago.BlockID) {
		Component.LogInfof("milestone issued (%d) MilestoneID: %s, BlockID: %v", index, iotago.EncodeHex(milestoneID[:]), blockID.ToHex())
	}

	unhook := lo.Batch(
		deps.TangleListener.Events.BlockSolid.Hook(onBlockSolid).Unhook,
		deps.NodeBridge.Events.LatestMilestoneChanged.Hook(onLatestMilestoneChanged).Unhook,
		deps.NodeBridge.Events.ConfirmedMilestoneChanged.Hook(onConfirmedMilestoneChanged).Unhook,
		deps.Coordinator.Events.IssuedCheckpointBlock.Hook(onIssuedCheckpoint).Unhook,
		deps.Coordinator.Events.IssuedMilestone.Hook(onIssuedMilestone).Unhook,
		deps.Coordinator.Events.MilestoneTimeout.Hook(onMilestoneTimeout).Unhook,
	)

	return unhook
}
