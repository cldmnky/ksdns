/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cldmnky/ksdns/pkg/zupd/config"
	"github.com/cldmnky/ksdns/pkg/zupd/server"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run called")
		config, err := config.NewConfig("127.0.0.1", "1053", "foo", []string{"example.com:example.com.db"}, "", "", "")
		if err != nil {
			cobra.CheckErr(err)
		}
		server := server.NewServer(config)

		ctx, cancel := context.WithCancel(cmd.Context())
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		defer func() {
			signal.Stop(c)
			cancel()
		}()
		go func() {
			select {
			case <-c:
				cancel()
			case <-ctx.Done():
			}
		}()
		server.Run(cmd.Context())
		<-ctx.Done()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
