package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spf13/cobra"
	bladeapiv1alpha1 "github.com/uptime-industries/compute-blade-agent/api/bladeapi/v1alpha1"
	"github.com/uptime-industries/compute-blade-agent/pkg/log"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"

	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	bladeapiv1alpha1 "github.com/uptime-industries/compute-blade-agent/api/bladeapi/v1alpha1"
	"github.com/uptime-industries/compute-blade-agent/cmd/bladectl/config"
	"github.com/uptime-industries/compute-blade-agent/pkg/log"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	bladeName string
	timeout   time.Duration
)

func init() {
	rootCmd.PersistentFlags().StringVar(&bladeName, "blade", "", "Name of the compute-blade to control. If not provided, the compute-blade specified in `current-blade` will be used.")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", time.Minute, "timeout for gRPC requests")
}

var rootCmd = &cobra.Command{
	Use:   "bladectl",
	Short: "bladectl interacts with the compute-blade-agent and allows you to manage hardware-features of your compute blade(s)",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		origCtx := cmd.Context()

		// Load potential file configs
		if err := viper.ReadInConfig(); err != nil {
			return err
		}

		// load configuration
		var bladectlCfg config.BladectlConfig
		if err := viper.Unmarshal(&bladectlCfg); err != nil {
			return err
		}

		var blade *config.Blade

		if len(bladeName) > 0 {
			var herr humane.Error
			if blade, herr = bladectlCfg.FindBlade(bladeName); herr != nil {
				return fmt.Errorf(herr.Display())
			}
		}

		blade, herr := bladectlCfg.FindBlade(bladeName)
		if herr != nil {
			return fmt.Errorf(herr.Display())
		}

		// setup signal handlers for SIGINT and SIGTERM
		ctx, cancelCtx := context.WithTimeout(origCtx, timeout)

		// setup signal handler channels
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			select {
			// Wait for context cancel
			case <-ctx.Done():

			// Wait for signal
			case sig := <-sigs:
				switch sig {
				case syscall.SIGTERM:
					fallthrough
				case syscall.SIGINT:
					fallthrough
				case syscall.SIGQUIT:
					// On terminate signal, cancel context causing the program to terminate
					cancelCtx()

				default:
					log.FromContext(ctx).Warn("Received unknown signal", zap.String("signal", sig.String()))
				}
			}
		}()

		// Create our gRPC Transport Credentials
		credentials := insecure.NewCredentials()
		conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(credentials))
		if err != nil {
			return fmt.Errorf(
				humane.Wrap(err,
					"failed to dial grpc server",
					"ensure the gRPC server you are trying to connect to is running and the address is correct",
				).Display(),
			)
		}

		client := bladeapiv1alpha1.NewBladeAgentServiceClient(conn)
		cmd.SetContext(clientIntoContext(ctx, client))
		return nil
	},
}
