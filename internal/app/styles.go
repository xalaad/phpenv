package app

import "github.com/fatih/color"

var (
	styleTitle    = color.New(color.FgCyan, color.Bold)
	styleSection  = color.New(color.FgHiCyan, color.Bold)
	styleInfo     = color.New(color.FgHiWhite)
	styleWarn     = color.New(color.FgYellow, color.Bold)
	styleError    = color.New(color.FgRed, color.Bold)
	styleSuccess  = color.New(color.FgGreen, color.Bold)
	styleList     = color.New(color.FgWhite)
	styleListBold = color.New(color.FgGreen, color.Bold)
	styleHelpKey  = color.New(color.FgHiBlue, color.Bold)
	styleHelpDesc = color.New(color.FgWhite)
)

func printInfo(format string, args ...interface{}) {
	styleInfo.Printf(format+"\n", args...)
}

func printWarn(format string, args ...interface{}) {
	styleWarn.Printf(format+"\n", args...)
}

func printSuccess(format string, args ...interface{}) {
	styleSuccess.Printf(format+"\n", args...)
}

func printError(format string, args ...interface{}) {
	styleError.Printf(format+"\n", args...)
}

func printTitle(format string, args ...interface{}) {
	styleTitle.Printf(format+"\n", args...)
}
