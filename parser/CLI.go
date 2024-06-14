package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
)

// Default Values
const (
	defaultAgentVariableName = "NewRelicAgent"
	defaultAppName           = "AST Example"
	defaultPackageName       = "."
	defaultPackagePath       = "../demo-app"
)

type CLIConfig struct {
	PackagePath       string
	PackageName       string
	AppName           string
	AgentVariableName string
	DiffFile          string
}

func CLISplash() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Printf("New Relic Go Agent Pre Instrumentation Tool (Pre-Alpha)\n")
	fmt.Printf("-------------------------------------------------------\n")
	fmt.Printf("This tool will instrument your Go application with the New Relic Go Agent\n")
	fmt.Printf("And generate a diff file to show the changes made to your code\n")
	fmt.Printf("-------------------------------------------------------\n")
	fmt.Printf("\n")
	fmt.Printf("\n")

}

func NewCLIConfig() *CLIConfig {
	return &CLIConfig{
		PackagePath:       defaultPackagePath,
		PackageName:       defaultPackageName,
		AppName:           defaultAppName,
		AgentVariableName: defaultAgentVariableName,
	}
}

func (cfg *CLIConfig) CLIPrompts() {

	cfg.PackagePath = defaultPackagePath
	cfg.PackageName = defaultPackageName
	cfg.AppName = defaultAppName
	cfg.AgentVariableName = defaultAgentVariableName

	wd, _ := os.Getwd()
	diffFileLocation := wd
	cfg.DiffFile = fmt.Sprintf("%s/%s.diff", diffFileLocation, path.Base(cfg.PackagePath))

	// wd, _ := os.Getwd()
	// diffFileLocation = wd

	// Prompt user to enter the path to the package they want to instrument
	var packagePathPrompt string
	fmt.Printf("Enter the path to the package you want to instrument (default if left blank: '%s'):", defaultPackagePath)
	fmt.Scanln(&packagePathPrompt)

	if packagePathPrompt != "" {
		_, err := os.Stat(packagePathPrompt)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			os.Exit(1)
		} else {
			cfg.PackagePath = packagePathPrompt
		}
	}

	// Prompt user to enter the package name
	// fmt.Printf("Enter the package name (default: '%s'):", defaultPackageName)
	// fmt.Scanln(&packageName)

	// if packageName == "" {
	//	packageName = defaultPackageName
	// }
	// Prompt user to enter the application name
	fmt.Printf("Override Application Name? Y/N (default: '%s'):", defaultAppName)
	userPrompt := ""
	fmt.Scanln(&userPrompt)

	if userPrompt == "Y" || userPrompt == "y" {
		fmt.Printf("Enter the application name:")
		var userAppName string
		fmt.Scanln(&userAppName)
		if userAppName != "" {
			cfg.AppName = userAppName
		}
	}

	// Prompt user to enter the diff file output location
	// fmt.Printf("Enter the diff file output location (default: '%s/'):", diffFileLocation)
	// fmt.Scanln(&diffFileLocation)

	// if diffFileLocation == "" {
	// Set default diff file output location
	// wd, _ := os.Getwd()
	// diffFileLocation = wd

	// }
}
