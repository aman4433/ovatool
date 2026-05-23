package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ppc64le-cloud/ovatool/pkg/config"
)

var (
	logger   *zap.Logger
	cfg      *config.Config
	jsonLogs bool
	logFile  string
)

var rootCmd = &cobra.Command{
	Use:   "ovatool",
	Short: "OVA image build, upload and import automation for PowerVS and PowerVC",
	Long: `ovatool automates the full lifecycle of OVA images for IBM Power:

  1. fetch   — resolve and download a prebuilt RHCOS OVA from Red Hat
  2. build   — convert a qcow2 image to OVA format using pvsadm
  3. upload  — push the OVA to an IBM Cloud Object Storage bucket
  4. import  — import from COS into PowerVS and/or PowerVC
  5. run     — orchestrate any combination of the above stages

After a successful import, ovatool prints a pre-formatted wiki update section
ready to paste into the team wiki page — no manual copy-paste errors.

Configuration is loaded from environment variables. Run 'ovatool init'
to generate a .env.template that you can fill in and rename to .env.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "init" {
			return nil
		}
		if err := config.LoadEnvFile(); err != nil {
			return fmt.Errorf("loading .env: %w", err)
		}
		cfg = config.FromEnv()
		logger = buildLogger()
		return nil
	},
}

// Execute is the entrypoint called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonLogs, "json-logs", false, "emit logs as JSON (useful for CI / log aggregators)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "write logs to this file in addition to stdout")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(runCmd)
}

// buildLogger constructs a zap logger based on global flags.
func buildLogger() *zap.Logger {
	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")

	var encoder zapcore.Encoder
	if jsonLogs {
		encoder = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	} else {
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	cores := []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel),
	}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open log file %s: %v\n", logFile, err)
		} else {
			fileEnc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
			cores = append(cores, zapcore.NewCore(fileEnc, zapcore.AddSync(f), zapcore.DebugLevel))
		}
	}

	return zap.New(zapcore.NewTee(cores...), zap.AddCaller())
}
