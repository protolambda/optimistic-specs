package flags

import "github.com/urfave/cli"

// Flags

const envVarPrefix = "ROLLUP_NODE_"

func prefixEnvVar(name string) string {
	return envVarPrefix + name
}

var (
	/* Required Flags */
	L1NodeAddr = cli.StringFlag{
		Name:     "l1",
		Usage:    "Address of L1 User JSON-RPC endpoint to use (eth namespace required)",
		Required: true,
		Value:    "http://127.0.0.1:8545",
		EnvVar:   prefixEnvVar("L1_ETH_RPC"),
	}
	L2EngineAddrs = cli.StringSliceFlag{
		Name:     "l2",
		Usage:    "Addresses of L2 Engine JSON-RPC endpoints to use (engine and eth namespace required)",
		Required: true,
		EnvVar:   prefixEnvVar("L2_ENGINE_RPC"),
	}
	RollupConfig = cli.StringFlag{
		Name:     "rollup.config",
		Usage:    "Rollup chain parameters",
		Required: true,
		EnvVar:   prefixEnvVar("ROLLUP_CONFIG"),
	}
	L2NodeAddr = cli.StringFlag{
		Name:     "eth.l2",
		Usage:    "Address of L2 User JSON-RPC endpoint to use (eth namespace required)",
		Required: true,
		EnvVar:   prefixEnvVar("L2_ETH_RPC"),
	}
	RPCListenAddr = cli.StringFlag{
		Name:     "rpc.addr",
		Usage:    "RPC listening address",
		Required: true,
		EnvVar:   prefixEnvVar("RPC_ADDR"),
	}
	RPCListenPort = cli.IntFlag{
		Name:     "rpc.port",
		Usage:    "RPC listening port",
		Required: true,
		EnvVar:   prefixEnvVar("RPC_PORT"),
	}
	WithdrawalContractAddr = cli.StringFlag{
		Name:     "rpc.withdrawalcontractaddress",
		Usage:    "Address of the Withdrawal contract. Used to serve RPC",
		Required: true,
		EnvVar:   prefixEnvVar("WITHDRAWAL_CONTRACT_ADDR"),
	}

	/* Optional Flags */

	SequencingEnabledFlag = cli.BoolFlag{
		Name:   "sequencing.enabled",
		Usage:  "enable sequencing",
		EnvVar: prefixEnvVar("SEQUENCING_ENABLED"),
	}

	// TODO: move batch submitter to stand-alone process
	BatchSubmitterKeyFlag = cli.StringFlag{
		Name:   "batchsubmitter.key",
		Usage:  "key for batch submitting",
		EnvVar: prefixEnvVar("BATCHSUBMITTER_KEY"),
	}

	LogLevelFlag = cli.StringFlag{
		Name:   "log.level",
		Usage:  "The lowest log level that will be output",
		Value:  "info",
		EnvVar: prefixEnvVar("LOG_LEVEL"),
	}
	LogFormatFlag = cli.StringFlag{
		Name:   "log.format",
		Usage:  "Format the log output. Supported formats: 'text', 'json'",
		Value:  "text",
		EnvVar: prefixEnvVar("LOG_FORMAT"),
	}
	LogColorFlag = cli.BoolFlag{
		Name:   "log.color",
		Usage:  "Color the log output",
		EnvVar: prefixEnvVar("LOG_COLOR"),
	}
)

var requiredFlags = []cli.Flag{
	L1NodeAddr,
	L2EngineAddrs,
	RollupConfig,
	L2NodeAddr,
	RPCListenAddr,
	RPCListenPort,
	WithdrawalContractAddr,
}

var optionalFlags = []cli.Flag{
	SequencingEnabledFlag,
	BatchSubmitterKeyFlag,
	LogLevelFlag,
	LogFormatFlag,
	LogColorFlag,
}

// Flags contains the list of configuration options available to the binary.
var Flags = append(requiredFlags, optionalFlags...)
