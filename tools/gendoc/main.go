package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/apputils/config"
	cooApp "github.com/iotaledger/inx-coordinator/core/app"
)

func createMarkdownFile(app *app.App, markdownHeaderPath string, markdownFilePath string, ignoreFlags map[string]struct{}, replaceTopicNames map[string]string) {

	markdownHeader := ""

	if markdownHeaderPath != "" {
		var err error
		markdownHeaderFile, err := os.ReadFile(markdownHeaderPath)
		if err != nil {
			panic(err)
		}

		markdownHeader = string(markdownHeaderFile)
	}

	if strings.HasPrefix(markdownHeader, "---") {
		// header contains frontmatter code block
		markdownHeader = strings.Replace(markdownHeader, "---", `---
# !!! DO NOT MODIFY !!!
# This file is auto-generated by the gendoc tool based on the source code of the app.`, 1)
	} else {
		markdownHeader = `<!---
!!! DO NOT MODIFY !!!

This file is auto-generated by the gendoc tool based on the source code of the app.
-->
` + markdownHeader
	}

	println(fmt.Sprintf("Create markdown file for %s...", app.Info().Name))
	md := config.GetConfigurationMarkdown(app.Config(), app.FlagSet(), ignoreFlags, replaceTopicNames)
	if err := os.WriteFile(markdownFilePath, append([]byte(markdownHeader), []byte(md)...), os.ModePerm); err != nil {
		panic(err)
	}
	println(fmt.Sprintf("Markdown file for %s stored: %s", app.Info().Name, markdownFilePath))
}

func createDefaultConfigFile(app *app.App, configFilePath string, ignoreFlags map[string]struct{}) {
	println(fmt.Sprintf("Create default configuration file for %s...", app.Info().Name))
	conf := config.GetDefaultAppConfigJSON(app.Config(), app.FlagSet(), ignoreFlags)
	if err := os.WriteFile(configFilePath, []byte(conf), os.ModePerm); err != nil {
		panic(err)
	}
	println(fmt.Sprintf("Default configuration file for %s stored: %s", app.Info().Name, configFilePath))
}

func main() {

	// MUST BE LOWER CASE
	ignoreFlags := make(map[string]struct{})

	replaceTopicNames := make(map[string]string)
	replaceTopicNames["app"] = "Application"
	replaceTopicNames["api"] = "API"
	replaceTopicNames["tipsel"] = "Tipselection"
	replaceTopicNames["inx"] = "INX"

	application := cooApp.App()

	createMarkdownFile(
		application,
		"configuration_header.md",
		"../../documentation/docs/configuration.md",
		ignoreFlags,
		replaceTopicNames,
	)

	createDefaultConfigFile(
		application,
		"../../config_defaults.json",
		ignoreFlags,
	)
}
