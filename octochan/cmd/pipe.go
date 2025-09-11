// pipe_executor.go
package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func ExecuteCommandWithPipe(command string, args []string, inputData []byte) ([]byte, error) {
	var inputReader io.Reader
	if len(inputData) > 0 {
		inputReader = bytes.NewReader(inputData)
	} else {
		inputReader = os.Stdin
	}

	cmd := exec.Command(os.Args[0], append([]string{command}, args...)...)

	cmd.Stdin = inputReader
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %v", command, err)
	}

	return output.Bytes(), nil
}

func ExecuteCommand(command string, args []string, input []byte) ([]byte, error) {
	return ExecuteCommandWithPipe(command, args, input)
}

func PipeWrapper(cmd *cobra.Command) *cobra.Command {

	originalRun := cmd.Run
	originalRunE := cmd.RunE

	wrappedRun := func(cmd *cobra.Command, args []string) {

		stat, _ := os.Stdin.Stat()
		hasStdin := (stat.Mode() & os.ModeCharDevice) == 0

		if hasStdin {
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				cmd.Printf("❌ Ошибка чтения stdin: %v\n", err)
				return
			}

			if canHandleStdin(cmd) {
				args = adaptArgsForStdin(cmd, args, stdinData)
			}
		}

		if originalRun != nil {
			originalRun(cmd, args)
		} else if originalRunE != nil {
			if err := originalRunE(cmd, args); err != nil {
				cmd.PrintErrln("❌", err)
			}
		}
	}

	cmd.Run = wrappedRun
	cmd.RunE = nil

	return cmd
}

func canHandleStdin(cmd *cobra.Command) bool {

	stdinCompatible := map[string]bool{
		"diff":     true,
		"apply":    true,
		"validate": true,
		"patch":    true,
		"find":     true,
		"stats":    true,
		"print":    true,
		"tee":      true,
	}

	return stdinCompatible[cmd.Name()]
}

func adaptArgsForStdin(cmd *cobra.Command, args []string, stdinData []byte) []string {
	switch cmd.Name() {
	case "diff":

		if len(args) == 1 {
			return append(args, "-")
		}
	case "apply":
		if len(args) == 0 {
			return []string{"-"}
		}
	case "validate", "stats", "find":
		// validate - , stats - , find - param
		if len(args) == 0 {
			return []string{"-"}
		} else if cmd.Name() == "find" && len(args) == 1 {
			return []string{"-", args[0]}
		}
	}

	return args
}
func IsPipeMode() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) == 0
}
