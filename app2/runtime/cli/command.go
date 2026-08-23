// Package cli adapts command-line configuration to the Runtime host.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/app2/runtime/agenttools"
	"github.com/Tangerg/lynx/app2/runtime/codeintel"
	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
)

const defaultListen = "127.0.0.1:17172"

type runtimeProcess interface {
	Run(context.Context) error
	Close(context.Context) error
	BaseURL() string
}

type dependencies struct {
	version     string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	userHomeDir func() (string, error)
	openRuntime func(context.Context, runtimehost.Config) (runtimeProcess, error)
}

func New(version string, stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	return newCommand(dependencies{
		version: version, stdin: stdin, stdout: stdout, stderr: stderr,
		userHomeDir: os.UserHomeDir,
		openRuntime: func(ctx context.Context, config runtimehost.Config) (runtimeProcess, error) {
			return runtimehost.Open(ctx, config)
		},
	})
}

func newCommand(deps dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "lyra-runtime",
		Short:         "Run the Lyra app2 Runtime",
		Version:       deps.version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.AddCommand(newServeCommand(deps))
	return root
}

func newServeCommand(deps dependencies) *cobra.Command {
	settings := viper.New()
	settings.SetConfigType("yaml")
	settings.SetEnvPrefix("LYRA2")
	settings.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	settings.SetDefault("listen", defaultListen)
	settings.SetDefault("serverName", "lyra-runtime")
	settings.SetDefault("corsOrigins", httptransport.DefaultCORSOrigins())
	settings.SetDefault("httpAllowedMethods", []string{"GET", "HEAD"})

	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve JSON-RPC and operational sidecars on loopback HTTP",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := readConfig(settings); err != nil {
				return err
			}
			config, err := resolveConfig(settings, deps)
			if err != nil {
				return err
			}
			return serve(command.Context(), deps, config)
		},
	}
	flags := command.Flags()
	flags.String("config", "", "absolute path to a YAML configuration file")
	flags.String("listen", "", "loopback listen address")
	flags.String("data-home", "", "absolute app2 Runtime data directory")
	flags.String("database-path", "", "absolute SQLite database path")
	flags.String("token-path", "", "absolute local bearer-token path")
	flags.Bool("no-local-token", false, "disable the local bearer-token gate")
	flags.String("bootstrap-descriptor", "", "absolute one-shot bootstrap descriptor path")
	flags.String("bootstrap-nonce", "", "nonce expected by the supervising Desktop")
	flags.String("workspace", "", "absolute default workspace path")
	flags.String("user-home", "", "absolute user home reported by discovery")
	flags.String("server-name", "", "Runtime server name")
	flags.StringSlice("cors-origin", nil, "exact browser origin allowed to access the Runtime")
	flags.StringSlice("http-allowed-host", nil, "host pattern the agent HTTP tool may reach")
	flags.StringSlice("http-allowed-method", nil, "HTTP method the agent HTTP tool may use")
	flags.Bool("remote", false, "serve an explicitly configured remote HTTPS endpoint")
	flags.String("tls-certificate", "", "absolute PEM certificate chain path for remote HTTPS")
	flags.String("tls-private-key", "", "absolute PEM private-key path for remote HTTPS")

	bindFlag(settings, flags, "config", "config")
	bindFlag(settings, flags, "listen", "listen")
	bindFlag(settings, flags, "dataHome", "data-home")
	bindFlag(settings, flags, "databasePath", "database-path")
	bindFlag(settings, flags, "tokenPath", "token-path")
	bindFlag(settings, flags, "noLocalToken", "no-local-token")
	bindFlag(settings, flags, "descriptorPath", "bootstrap-descriptor")
	bindFlag(settings, flags, "bootstrapNonce", "bootstrap-nonce")
	bindFlag(settings, flags, "workspace", "workspace")
	bindFlag(settings, flags, "userHome", "user-home")
	bindFlag(settings, flags, "serverName", "server-name")
	bindFlag(settings, flags, "corsOrigins", "cors-origin")
	bindFlag(settings, flags, "httpAllowedHosts", "http-allowed-host")
	bindFlag(settings, flags, "httpAllowedMethods", "http-allowed-method")
	bindFlag(settings, flags, "remote", "remote")
	bindFlag(settings, flags, "tlsCertificatePath", "tls-certificate")
	bindFlag(settings, flags, "tlsPrivateKeyPath", "tls-private-key")
	for key, environment := range map[string]string{
		"listen": "LYRA2_LISTEN", "dataHome": "LYRA2_DATA_HOME", "databasePath": "LYRA2_DATABASE_PATH",
		"tokenPath": "LYRA2_TOKEN_PATH", "noLocalToken": "LYRA2_NO_LOCAL_TOKEN",
		"descriptorPath": "LYRA2_BOOTSTRAP_DESCRIPTOR", "bootstrapNonce": "LYRA2_BOOTSTRAP_NONCE",
		"workspace": "LYRA2_WORKSPACE", "userHome": "LYRA2_USER_HOME", "serverName": "LYRA2_SERVER_NAME",
		"corsOrigins": "LYRA2_CORS_ORIGINS",
		"jinaAPIKey":  "LYRA2_JINA_API_KEY", "tavilyAPIKey": "LYRA2_TAVILY_API_KEY",
		"httpAllowedHosts": "LYRA2_HTTP_ALLOWED_HOSTS", "httpAllowedMethods": "LYRA2_HTTP_ALLOWED_METHODS",
		"remote": "LYRA2_REMOTE", "tlsCertificatePath": "LYRA2_TLS_CERTIFICATE", "tlsPrivateKeyPath": "LYRA2_TLS_PRIVATE_KEY",
	} {
		if err := settings.BindEnv(key, environment); err != nil {
			panic(err)
		}
	}
	return command
}

func bindFlag(settings *viper.Viper, flags *pflag.FlagSet, key, name string) {
	if err := settings.BindPFlag(key, flags.Lookup(name)); err != nil {
		panic(err)
	}
}

func readConfig(settings *viper.Viper) error {
	path := settings.GetString("config")
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return errors.New("cli: --config must be an absolute path")
	}
	settings.SetConfigFile(filepath.Clean(path))
	if err := settings.ReadInConfig(); err != nil {
		return fmt.Errorf("cli: read config: %w", err)
	}
	return nil
}

func resolveConfig(settings *viper.Viper, deps dependencies) (runtimehost.Config, error) {
	userHome := settings.GetString("userHome")
	if userHome == "" {
		resolved, err := deps.userHomeDir()
		if err != nil {
			return runtimehost.Config{}, fmt.Errorf("cli: locate user home: %w", err)
		}
		userHome = resolved
	}
	workspace := settings.GetString("workspace")
	if workspace == "" {
		workspace = userHome
	}
	dataHome := settings.GetString("dataHome")
	if dataHome == "" {
		dataHome = filepath.Join(userHome, ".lyra-app2")
	}
	databasePath := settings.GetString("databasePath")
	if databasePath == "" {
		databasePath = filepath.Join(dataHome, "runtime.sqlite")
	}
	tokenPath := settings.GetString("tokenPath")
	if settings.GetBool("noLocalToken") {
		tokenPath = ""
	} else if tokenPath == "" {
		tokenPath = filepath.Join(dataHome, "local-token")
	}
	lspServers, err := resolveLSPServers(settings)
	if err != nil {
		return runtimehost.Config{}, err
	}
	return runtimehost.Config{
		Listen: settings.GetString("listen"), DatabasePath: filepath.Clean(databasePath), TokenPath: cleanOptional(tokenPath),
		DescriptorPath: cleanOptional(settings.GetString("descriptorPath")), BootstrapNonce: settings.GetString("bootstrapNonce"),
		DefaultWorkspace: filepath.Clean(workspace), UserHome: filepath.Clean(userHome),
		ServerName: settings.GetString("serverName"), ServerVersion: deps.version,
		CORSOrigins: settings.GetStringSlice("corsOrigins"),
		Online: agenttools.OnlineConfig{
			JinaAPIKey: settings.GetString("jinaAPIKey"), TavilyAPIKey: settings.GetString("tavilyAPIKey"),
			HTTPAllowedHosts:   settings.GetStringSlice("httpAllowedHosts"),
			HTTPAllowedMethods: settings.GetStringSlice("httpAllowedMethods"),
		},
		LSPServers:         lspServers,
		Remote:             settings.GetBool("remote"),
		TLSCertificatePath: cleanOptional(settings.GetString("tlsCertificatePath")),
		TLSPrivateKeyPath:  cleanOptional(settings.GetString("tlsPrivateKeyPath")),
	}, nil
}

type lspServerConfig struct {
	Name        string   `mapstructure:"name"`
	Command     string   `mapstructure:"command"`
	Args        []string `mapstructure:"args"`
	LanguageID  string   `mapstructure:"languageId"`
	Extensions  []string `mapstructure:"extensions"`
	RootMarkers []string `mapstructure:"rootMarkers"`
}

func resolveLSPServers(settings *viper.Viper) ([]codeintel.ServerSpec, error) {
	var configured []lspServerConfig
	if err := settings.UnmarshalKey("lsp.servers", &configured); err != nil {
		return nil, fmt.Errorf("cli: decode lsp.servers: %w", err)
	}
	servers := make([]codeintel.ServerSpec, len(configured))
	for index, value := range configured {
		servers[index] = codeintel.ServerSpec{
			Name: value.Name, Command: value.Command, Args: value.Args,
			LanguageID: value.LanguageID, Extensions: value.Extensions,
			RootMarkers: value.RootMarkers,
		}
	}
	return servers, nil
}

func cleanOptional(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func serve(ctx context.Context, deps dependencies, config runtimehost.Config) (err error) {
	logger := slog.New(slog.NewJSONHandler(deps.stderr, nil))
	config.Logger = logger
	process, err := deps.openRuntime(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		err = errors.Join(err, process.Close(closeCtx))
	}()
	logger.Info("Runtime starting", "baseURL", process.BaseURL(), "protocolVersion", protocol.ProtocolVersion)
	return process.Run(ctx)
}
