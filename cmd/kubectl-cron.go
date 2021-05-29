package main

import (
	"os"

	"github.com/aviklai/kubectl-cron/pkg/cmd"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-cron", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewCmdCron(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
