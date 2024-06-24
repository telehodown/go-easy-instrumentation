package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Default Values
const (
	defaultAgentVariableName = "NewRelicAgent"
	defaultPackageName       = "."
	defaultPackagePath       = "../demo-app"
	defaultDiffFileName      = "new-relic-instrumentation.diff"
)

type CLIConfig struct {
	PackagePath       string
	PackageName       string
	AppName           string
	AgentVariableName string
	DiffFile          string
}

func CLISplash() {
	fmt.Printf("\n")
	fmt.Printf("      New Relic Go Agent Assisted Instrumentation Alpha\n")
	fmt.Printf("--------------------------------------------------------------\n")
	fmt.Printf("This tool will generate a diff file containing changes that\n")
	fmt.Printf("instrument your Go application with the New Relic Go Agent\n")
	fmt.Printf("--------------------------------------------------------------\n")
	fmt.Printf("\n")
	fmt.Printf("\n")

}

func NewCLIConfig() *CLIConfig {
	wd, _ := os.Getwd()
	diffFileLocation := wd
	diffFile := fmt.Sprintf("%s/%s.diff", diffFileLocation, path.Base(defaultPackagePath))

	return &CLIConfig{
		PackagePath:       defaultPackagePath,
		PackageName:       defaultPackageName,
		AgentVariableName: defaultAgentVariableName,
		DiffFile:          diffFile,
	}
}

func (cfg *CLIConfig) CLIPrompts() {

	cfg.PackagePath = defaultPackagePath
	cfg.PackageName = defaultPackageName
	cfg.AgentVariableName = defaultAgentVariableName
	wd, _ := os.Getwd()
	// Prompt user to enter the path to the package they want to instrument
	var packagePathPrompt string
	fmt.Printf("Enter the path to the application you want to instrument (default: '%s'): ", defaultPackagePath)
	fmt.Scanln(&packagePathPrompt)

	if packagePathPrompt != "" {
		_, err := os.Stat(packagePathPrompt)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			os.Exit(1)
		} else {
			// Set new path and update diff file name
			cfg.PackagePath = strings.TrimSpace(packagePathPrompt)
			cfg.DiffFile = fmt.Sprintf("%s/%s.diff", wd, path.Base(cfg.PackagePath))

		}
	}
	fmt.Printf("\tThis tool will generate instrumentation for: %s\n", cfg.PackagePath)

	// Prompt user to enter the package name
	// fmt.Printf("Enter the package name (default: '%s'):", defaultPackageName)
	// fmt.Scanln(&packageName)

	// if packageName == "" {
	//	packageName = defaultPackageName
	// }
	// Prompt user to enter the application name
	fmt.Printf("Override the New Relic application name? Y/N: ")
	userPrompt := ""
	fmt.Scanln(&userPrompt)

	if userPrompt == "Y" || userPrompt == "y" {
		fmt.Printf("Enter the application name: ")
		var userAppName string
		fmt.Scanln(&userAppName)
		if userAppName != "" {
			userAppName = strings.TrimSpace(userAppName)
			cfg.AppName = userAppName
		}
	}

	// Prompt user to enter the diff file output location
	localFile, _ := filepath.Rel(wd, cfg.DiffFile)
	fmt.Printf("Would you like to change the location of the diff file (default: \"%s\")? Y/N: ", localFile)
	userPrompt = ""
	fmt.Scanln(&userPrompt)
	if userPrompt == "Y" || userPrompt == "y" {
		fmt.Printf("What directory will the diff file be written to: ")
		diffDirectory := ""
		fmt.Scanln(&diffDirectory)
		diffDirectory = strings.TrimSpace(diffDirectory)
		if diffDirectory == "" {
			diffDirectory = wd
		}

		_, err := os.Stat(diffDirectory)
		if err != nil {
			log.Fatalf("The path \"%s\" could not be found: %v", diffDirectory, err)
		}

		fmt.Printf("\tThe diff file will be written in the directory: \"%s\"\n", diffDirectory)

		fmt.Printf("What woud you like to name your diff file (default: \"%s\"): ", defaultDiffFileName)
		diffFileName := ""
		fmt.Scanln(&diffFileName)
		diffFileName = strings.TrimSpace(diffFileName)
		if diffFileName == "" {
			diffFileName = defaultDiffFileName
		}

		ext := filepath.Ext(diffFileName)
		if ext == "" {
			diffFileName = diffFileName + ".diff"
		} else if ext != ".diff" {
			fmt.Println(ext)
			diffFileName = strings.TrimSuffix(diffFileName, ext) + ".diff"
		}
		cfg.DiffFile = filepath.Join(diffDirectory, diffFileName)
	}

	fmt.Printf("\tThe diff file will be written at: \"%s\"\n", cfg.DiffFile)
}
