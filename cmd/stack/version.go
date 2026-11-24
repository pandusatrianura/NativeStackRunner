package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display binary version, commit, and platform information",
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Native Stack Runner (stack) version %s\n", Version)
			fmt.Fprintf(out, "  Commit:    %s\n", Commit)
			fmt.Fprintf(out, "  Built:     %s\n", BuildDate)
			fmt.Fprintf(out, "  OS/Arch:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
			fmt.Fprintf(out, "  Go:        %s\n", runtime.Version())
		},
	}
}
