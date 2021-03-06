---
description: This section describes the configuration parameters and their types for INX-Coordinator.
keywords:
- IOTA Node 
- Hornet Node
- Coordinator
- Configuration
- JSON
- Customize
- Config
- reference
---


# Core Configuration

INX-Coordinator uses a JSON standard format as a config file. If you are unsure about JSON syntax, you can find more information in the [official JSON specs](https://www.json.org).

You can change the path of the config file by using the `-c` or `--config` argument while executing `inx-coordinator` executable.

For example:
```bash
inx-coordinator -c config_defaults.json
```

You can always get the most up-to-date description of the config parameters by running:

```bash
inx-coordinator -h --full
```

## <a id="app"></a> 1. Application

| Name            | Description                                                                                            | Type    | Default value |
| --------------- | ------------------------------------------------------------------------------------------------------ | ------- | ------------- |
| checkForUpdates | Whether to check for updates of the application or not                                                 | boolean | true          |
| stopGracePeriod | The maximum time to wait for background processes to finish during shutdown before terminating the app | string  | "5m"          |

Example:

```json
  {
    "app": {
      "checkForUpdates": true,
      "stopGracePeriod": "5m"
    }
  }
```

## <a id="inx"></a> 2. INX

| Name    | Description                            | Type   | Default value    |
| ------- | -------------------------------------- | ------ | ---------------- |
| address | The INX address to which to connect to | string | "localhost:9029" |

Example:

```json
  {
    "inx": {
      "address": "localhost:9029"
    }
  }
```

## <a id="coordinator"></a> 3. Coordinator

| Name                                    | Description                                   | Type   | Default value       |
| --------------------------------------- | --------------------------------------------- | ------ | ------------------- |
| stateFilePath                           | The path to the state file of the coordinator | string | "coordinator.state" |
| interval                                | The interval milestones are issued            | string | "5s"                |
| [signing](#coordinator_signing)         | Configuration for signing                     | object |                     |
| [quorum](#coordinator_quorum)           | Configuration for quorum                      | object |                     |
| [checkpoints](#coordinator_checkpoints) | Configuration for checkpoints                 | object |                     |
| [tipsel](#coordinator_tipsel)           | Configuration for Tipselection                | object |                     |

### <a id="coordinator_signing"></a> Signing

| Name          | Description                                                                    | Type   | Default value     |
| ------------- | ------------------------------------------------------------------------------ | ------ | ----------------- |
| provider      | The signing provider the coordinator uses to sign a milestone (local/remote)   | string | "local"           |
| remoteAddress | The address of the remote signing provider (insecure connection!)              | string | "localhost:12345" |
| retryTimeout  | Defines the timeout between signing retries                                    | string | "2s"              |
| retryAmount   | Defines the number of signing retries to perform before shutting down the node | int    | 10                |

### <a id="coordinator_quorum"></a> Quorum

| Name    | Description                                                                                    | Type    | Default value     |
| ------- | ---------------------------------------------------------------------------------------------- | ------- | ----------------- |
| enabled | Whether the coordinator quorum is enabled                                                      | boolean | false             |
| timeout | The timeout until a node in the quorum must have answered                                      | string  | "2s"              |
| groups  | Defines the quorum groups used to ask other nodes for correct ledger state of the coordinator. | object  | see example below |

### <a id="coordinator_checkpoints"></a> Checkpoints

| Name             | Description                                                                                                       | Type | Default value |
| ---------------- | ----------------------------------------------------------------------------------------------------------------- | ---- | ------------- |
| maxTrackedBlocks | Maximum amount of known blocks for milestone tipselection. If this limit is exceeded, a new checkpoint is issued. | int  | 10000         |

### <a id="coordinator_tipsel"></a> Tipselection

| Name                                         | Description                                                                                                                                                                    | Type   | Default value |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------ | ------------- |
| minHeaviestBranchUnreferencedBlocksThreshold | Minimum threshold of unreferenced blocks in the heaviest branch                                                                                                                | int    | 20            |
| maxHeaviestBranchTipsPerCheckpoint           | Maximum amount of checkpoint blocks with heaviest branch tips that are picked if the heaviest branch is not below 'MinHeaviestBranchUnreferencedBlocksThreshold' before        | int    | 10            |
| randomTipsPerCheckpoint                      | Amount of checkpoint blocks with random tips that are picked if a checkpoint is issued and at least one heaviest branch tip was found, otherwise no random tips will be picked | int    | 3             |
| heaviestBranchSelectionTimeout               | The maximum duration to select the heaviest branch tips                                                                                                                        | string | "100ms"       |

Example:

```json
  {
    "coordinator": {
      "stateFilePath": "coordinator.state",
      "interval": "5s",
      "signing": {
        "provider": "local",
        "remoteAddress": "localhost:12345",
        "retryTimeout": "2s",
        "retryAmount": 10
      },
      "quorum": {
        "enabled": false,
        "timeout": "2s",
        "groups": {}
      },
      "checkpoints": {
        "maxTrackedBlocks": 10000
      },
      "tipsel": {
        "minHeaviestBranchUnreferencedBlocksThreshold": 20,
        "maxHeaviestBranchTipsPerCheckpoint": 10,
        "randomTipsPerCheckpoint": 3,
        "heaviestBranchSelectionTimeout": "100ms"
      }
    }
  }
```

## <a id="migrator"></a> 4. Migrator

| Name                | Description                                                                                                           | Type    | Default value    |
| ------------------- | --------------------------------------------------------------------------------------------------------------------- | ------- | ---------------- |
| enabled             | Whether the migrator plugin is enabled                                                                                | boolean | false            |
| stateFilePath       | Path to the state file of the migrator                                                                                | string  | "migrator.state" |
| receiptMaxEntries   | The max amount of entries to embed within a receipt                                                                   | int     | 110              |
| queryCooldownPeriod | The cooldown period for the service to ask for new data from the legacy node in case the migrator encounters an error | string  | "5s"             |

Example:

```json
  {
    "migrator": {
      "enabled": false,
      "stateFilePath": "migrator.state",
      "receiptMaxEntries": 110,
      "queryCooldownPeriod": "5s"
    }
  }
```

## <a id="receipts"></a> 5. Receipts

| Name                             | Description                 | Type   | Default value |
| -------------------------------- | --------------------------- | ------ | ------------- |
| [validator](#receipts_validator) | Configuration for validator | object |               |

### <a id="receipts_validator"></a> Validator

| Name                                           | Description                   | Type   | Default value |
| ---------------------------------------------- | ----------------------------- | ------ | ------------- |
| [api](#receipts_validator_api)                 | Configuration for API         | object |               |
| [coordinator](#receipts_validator_coordinator) | Configuration for coordinator | object |               |

### <a id="receipts_validator_api"></a> API

| Name    | Description                                                              | Type   | Default value            |
| ------- | ------------------------------------------------------------------------ | ------ | ------------------------ |
| address | Address of the legacy node API to query for white-flag confirmation data | string | "http://localhost:14266" |
| timeout | Timeout of API calls                                                     | string | "5s"                     |

### <a id="receipts_validator_coordinator"></a> Coordinator

| Name            | Description                                 | Type   | Default value                                                                       |
| --------------- | ------------------------------------------- | ------ | ----------------------------------------------------------------------------------- |
| address         | Address of the legacy coordinator           | string | "UDYXTZBE9GZGPM9SSQV9LTZNDLJIZMPUVVXYXFYVBLIEUHLSEWFTKZZLXYRHHWVQV9MNNX9KZC9D9UZWZ" |
| merkleTreeDepth | Depth of the Merkle tree of the coordinator | int    | 24                                                                                  |

Example:

```json
  {
    "receipts": {
      "validator": {
        "api": {
          "address": "http://localhost:14266",
          "timeout": "5s"
        },
        "coordinator": {
          "address": "UDYXTZBE9GZGPM9SSQV9LTZNDLJIZMPUVVXYXFYVBLIEUHLSEWFTKZZLXYRHHWVQV9MNNX9KZC9D9UZWZ",
          "merkleTreeDepth": 24
        }
      }
    }
  }
```

## <a id="profiling"></a> 6. Profiling

| Name        | Description                                       | Type    | Default value    |
| ----------- | ------------------------------------------------- | ------- | ---------------- |
| enabled     | Whether the profiling plugin is enabled           | boolean | false            |
| bindAddress | The bind address on which the profiler listens on | string  | "localhost:6060" |

Example:

```json
  {
    "profiling": {
      "enabled": false,
      "bindAddress": "localhost:6060"
    }
  }
```

