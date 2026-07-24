package cmd

import (
	"log/slog"
	"os"

	"github.com/Ericson246/npu-optimize/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile           string
	token             string
	modelDir          string
	outputFormat      string
	outputSchemaVer   int
	verbose           int
	llamaBenchVersion string
	logFormat         string
	runtimeCatalogURL string
)

var rootCmd = &cobra.Command{
	Use:   "npu-optimize",
	Short: "Detect hardware, find optimal GGUF models, and tune inference parameters",
	Long: `npu-optimize detects your hardware, queries HuggingFace for GGUF models,
calculates optimal inference configuration for llama.cpp, and optionally
runs benchmarks to validate performance.

Subcommands:
  detect    Dry-run: detect hardware + recommend model (no downloads)`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, initLogger)

	pf := rootCmd.PersistentFlags()

	pf.StringVarP(&token, "token", "t", "", "HuggingFace token (also reads HF_TOKEN, NPU_OPTIMIZE_TOKEN)")
	pf.StringVar(&modelDir, "model-dir", "./models", "Directory for model files")
	pf.StringVarP(&outputFormat, "output", "o", "json", "Output format: json or text")
	pf.IntVar(&outputSchemaVer, "output-schema-version", 1, "Requested output schema version")
	pf.CountVarP(&verbose, "verbose", "v", "Verbosity level (-v, -vv, -vvv)")
	pf.StringVar(&cfgFile, "config", "", "Path to config file")
	pf.StringVar(&llamaBenchVersion, "llama-bench-version", "b9180", "llama-bench version to use")
	pf.StringVar(&logFormat, "log-format", "text", "Log format: text or json")
	pf.StringVar(&runtimeCatalogURL, "runtime-catalog-url", "", "Override runtime catalog URL (empty uses embedded catalog)")

	_ = viper.BindPFlag("token", pf.Lookup("token"))
	_ = viper.BindPFlag("model_dir", pf.Lookup("model-dir"))
	_ = viper.BindPFlag("output", pf.Lookup("output"))
	_ = viper.BindPFlag("output_schema_version", pf.Lookup("output-schema-version"))
	_ = viper.BindPFlag("verbose", pf.Lookup("verbose"))
	_ = viper.BindPFlag("llama_bench_version", pf.Lookup("llama-bench-version"))
	_ = viper.BindPFlag("runtime_catalog_url", pf.Lookup("runtime-catalog-url"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.npu-optimize")
		viper.AddConfigPath("$HOME/.config/npu-optimize")
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("NPU_OPTIMIZE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			slog.Error("error reading config file", "err", err)
		}
	}
}

func initLogger() {
	logger.Init(logger.Config{
		Level:  verbose,
		Format: logFormat,
	})
}

func getToken() string {
	if token != "" {
		return token
	}
	if t := os.Getenv("HF_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("NPU_OPTIMIZE_TOKEN")
}

func getRuntimeCatalogURL() string {
	if runtimeCatalogURL != "" {
		return runtimeCatalogURL
	}
	if u := os.Getenv("NPU_OPTIMIZE_RUNTIME_CATALOG_URL"); u != "" {
		return u
	}
	return ""
}
