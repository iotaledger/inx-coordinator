package app

import (
	"github.com/iotaledger/hive.go/core/app"
	"github.com/iotaledger/hive.go/core/app/core/shutdown"
	"github.com/iotaledger/hive.go/core/app/plugins/profiling"
	"github.com/iotaledger/inx-app/inx"
	"github.com/iotaledger/inx-coordinator/core/coordinator"
	"github.com/iotaledger/inx-coordinator/plugins/migrator"
)

var (
	// Name of the app.
	Name = "inx-coordinator"

	// Version of the app.
	Version = "1.0.0-beta.9"
)

func App() *app.App {
	return app.New(Name, Version,
		app.WithInitComponent(InitComponent),
		app.WithCoreComponents([]*app.CoreComponent{
			inx.CoreComponent,
			coordinator.CoreComponent,
			shutdown.CoreComponent,
		}...),
		app.WithPlugins([]*app.Plugin{
			migrator.Plugin,
			profiling.Plugin,
			//nolint:gocritic // false positive
			//prometheus.Plugin,
		}...),
	)
}

var (
	InitComponent *app.InitComponent
)

func init() {
	InitComponent = &app.InitComponent{
		Component: &app.Component{
			Name: "App",
		},
		NonHiddenFlags: []string{
			"config",
			"help",
			"version",
			"migratorBootstrap",
			"migratorStartIndex",
			"cooBootstrap",
			"cooStartIndex",
		},
	}
}
