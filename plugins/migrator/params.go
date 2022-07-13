package migrator

import (
	"time"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-coordinator/pkg/migrator"
)

// ParametersMigrator contains the definition of the parameters used by Migrator.
type ParametersMigrator struct {
	// Enabled defines whether the migrator plugin is enabled.
	Enabled bool `default:"false" usage:"whether the migrator plugin is enabled"`
	// StateFilePath defines the path to the state file of the migrator.
	StateFilePath string `default:"migrator.state" usage:"path to the state file of the migrator"`
	// ReceiptMaxEntries defines the max amount of entries to embed within a receipt.
	ReceiptMaxEntries int `usage:"the max amount of entries to embed within a receipt"`
	// QueryCooldownPeriod defines the cooldown period for the service to ask for new data from the legacy node in case the migrator encounters an error.
	QueryCooldownPeriod time.Duration `default:"5s" usage:"the cooldown period for the service to ask for new data from the legacy node in case the migrator encounters an error"`
}

var ParamsMigrator = &ParametersMigrator{
	ReceiptMaxEntries: migrator.SensibleMaxEntriesCount,
}

// ParametersReceipts contains the definition of the parameters used by Receipts.
type ParametersReceipts struct {
	Validator struct {
		API struct {
			// Address defines the address of the legacy node API to query for white-flag confirmation data.
			Address string `default:"http://localhost:14266" usage:"address of the legacy node API to query for white-flag confirmation data"`
			// Address defines the timeout of API calls.
			Timeout time.Duration `default:"5s" usage:"timeout of API calls"`
		} `name:"api"`
		Coordinator struct {
			// Address defines the address of the legacy coordinator.
			Address string `default:"UDYXTZBE9GZGPM9SSQV9LTZNDLJIZMPUVVXYXFYVBLIEUHLSEWFTKZZLXYRHHWVQV9MNNX9KZC9D9UZWZ" usage:"address of the legacy coordinator"`
			// MerkleTreeDepth defines the depth of the Merkle tree of the coordinator.
			MerkleTreeDepth int `default:"24" usage:"depth of the Merkle tree of the coordinator"`
		}
	}
}

var ParamsReceipts = &ParametersReceipts{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"migrator": ParamsMigrator,
		"receipts": ParamsReceipts,
	},
	Masked: nil,
}
