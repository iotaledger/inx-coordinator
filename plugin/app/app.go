package app

import (
	flag "github.com/spf13/pflag"
	"go.uber.org/dig"

	"github.com/gohornet/hornet/pkg/node"
	"github.com/iotaledger/hive.go/configuration"
	"github.com/iotaledger/hive.go/logger"
)

var (
	nodeConfig = configuration.New()
	InitPlugin *node.InitPlugin
)

func init() {
	InitPlugin = &node.InitPlugin{
		Pluggable: node.Pluggable{
			Name:           "App",
			Params:         params,
			InitConfigPars: initConfigPars,
		},
		Configs: map[string]*configuration.Configuration{
			"nodeConfig": nodeConfig,
		},
		Init: initialize,
	}
}

func initialize(params map[string][]*flag.FlagSet, maskedKeys []string) (*node.InitConfig, error) {

	flagSets, err := normalizeFlagSets(params)
	if err != nil {
		return nil, err
	}

	parseFlags(flagSets)

	//TODO: load config file properly
	if err := nodeConfig.LoadFlagSet(flagSets["nodeConfig"]); err != nil {
		return nil, err
	}

	if err := logger.InitGlobalLogger(nodeConfig); err != nil {
		panic(err)
	}

	return &node.InitConfig{
		EnabledPlugins:  nodeConfig.Strings(CfgNodeEnablePlugins),
		DisabledPlugins: nodeConfig.Strings(CfgNodeDisablePlugins),
	}, nil
}

// adds the given flag sets to flag.CommandLine and then parses them.
func parseFlags(flagSets map[string]*flag.FlagSet) {
	for _, flagSet := range flagSets {
		flag.CommandLine.AddFlagSet(flagSet)
	}
	flag.Parse()
}

func normalizeFlagSets(params map[string][]*flag.FlagSet) (map[string]*flag.FlagSet, error) {
	fs := make(map[string]*flag.FlagSet)
	for cfgName, flagSets := range params {

		flagsUnderSameCfg := flag.NewFlagSet("", flag.ContinueOnError)
		for _, flagSet := range flagSets {
			flagSet.VisitAll(func(f *flag.Flag) {
				flagsUnderSameCfg.AddFlag(f)
			})
		}
		fs[cfgName] = flagsUnderSameCfg
	}
	return fs, nil
}

func initConfigPars(c *dig.Container) {

	type cfgResult struct {
		dig.Out
		NodeConfig *configuration.Configuration `name:"nodeConfig"`
	}

	if err := c.Provide(func() cfgResult {
		return cfgResult{
			NodeConfig: nodeConfig,
		}
	}); err != nil {
		InitPlugin.LogPanic(err)
	}
}
