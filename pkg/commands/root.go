package commands

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/yhlooo/pat/pkg/configs"
	"github.com/yhlooo/pat/pkg/i18n"
	"github.com/yhlooo/pat/pkg/polymarket"
	"github.com/yhlooo/pat/pkg/trading"
	"github.com/yhlooo/pat/pkg/trading/strategies"
	tradingui "github.com/yhlooo/pat/pkg/ui/trading"
	"github.com/yhlooo/pat/pkg/version"
)

// NewGlobalOptions 创建默认 GlobalOptions
func NewGlobalOptions() GlobalOptions {
	homeDir, _ := os.UserHomeDir()
	return GlobalOptions{
		Verbosity: 0,
		DataRoot:  filepath.Join(homeDir, ".pat"),
		Language:  "",
	}
}

// GlobalOptions 全局选项
type GlobalOptions struct {
	// 日志数量级别（ 0 / 1 / 2 ）
	Verbosity uint32
	// 数据存储根目录
	DataRoot string
	// 语言
	Language string
}

// Validate 校验选项是否合法
func (o *GlobalOptions) Validate() error {
	if o.Verbosity > 2 {
		return fmt.Errorf("invalid log verbosity: %d (expected: 0, 1 or 2)", o.Verbosity)
	}
	return nil
}

// AddPFlags 将选项绑定到命令行参数
func (o *GlobalOptions) AddPFlags(fs *pflag.FlagSet) {
	fs.Uint32VarP(&o.Verbosity, "verbose", "v", o.Verbosity, i18n.T(MsgGlobalOptsVerbosityDesc))
	fs.StringVar(&o.DataRoot, "data-root", o.DataRoot, i18n.T(MsgGlobalOptsDataRootDesc))
	fs.StringVar(&o.Language, "lang", o.Language, i18n.T(MsgGlobalOptsLangDesc))
}

// NewOptions 创建默认 Options
func NewOptions() Options {
	return Options{
		DryRun:   true,
		Scale:    1,
		Strategy: "discard",
	}
}

// Options 运行选项
type Options struct {
	DryRun   bool
	Scale    int
	Strategy string
}

// AddPFlags 将选项绑定到命令行参数
func (o *Options) AddPFlags(fs *pflag.FlagSet) {
	fs.BoolVar(
		&o.DryRun, "dry-run", o.DryRun,
		"The simulation runs and outputs the profit/loss results, but no actual transactions are made",
	)
	fs.IntVar(&o.Scale, "scale", o.Scale, "Transaction volume scaling factor")
	fs.StringVarP(&o.Strategy, "strategy", "s", o.Strategy, "Trading Strategy (one of 'discard', 'monkey', 'randwalk')")
}

// exampleTpl 运行示例说明模版
var exampleTpl = template.Must(template.New("Example").Parse(`
# Start trading on series
#
# Available Series:
# - btc-updown-5m:   Bitcoin Up or Down - 5 Minutes
# - btc-updown-15m:  Bitcoin Up or Down - 15 Minutes
# - eth-updown-5m:   Ethereum Up or Down - 5 Minutes
# - eth-updown-15m:  Ethereum Up or Down - 15 Minutes
{{ .CommandName }} btc-updown-5m

# Dry run
{{ .CommandName }} btc-updown-5m --dry-run
`))

// NewCommand 创建根命令
func NewCommand(name string) *cobra.Command {
	exampleBuf := new(bytes.Buffer)
	_ = exampleTpl.Execute(exampleBuf, map[string]any{
		"CommandName": name,
	})

	globalOpts := NewGlobalOptions()
	opts := NewOptions()

	var keylog *os.File
	cmd := &cobra.Command{
		Use:           fmt.Sprintf("%s [SERIES_SLUG]", name),
		Short:         i18n.T(MsgCmdShortDesc),
		Example:       exampleBuf.String(),
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if err := globalOpts.Validate(); err != nil {
				return err
			}

			// 创建日志目录
			if err := os.MkdirAll(globalOpts.DataRoot, 0o755); err != nil {
				return fmt.Errorf("create log directory %q error: %w", globalOpts.DataRoot, err)
			}

			// 初始化 logger
			logrusLogger := logrus.New()
			logrusLogger.SetOutput(&lumberjack.Logger{
				Filename:   filepath.Join(globalOpts.DataRoot, "pat.log"),
				MaxSize:    500, // MB
				MaxBackups: 3,
				MaxAge:     28, // 天
			})
			switch globalOpts.Verbosity {
			case 0:
				logrusLogger.Level = logrus.InfoLevel
			case 1:
				logrusLogger.Level = logrus.DebugLevel
			default:
				logrusLogger.Level = logrus.TraceLevel
			}
			logger := logrusr.New(logrusLogger)
			ctx = logr.NewContext(ctx, logger)

			// 加载配置
			cfgPath := filepath.Join(globalOpts.DataRoot, "pat.json")
			cfg, err := configs.LoadConfig(cfgPath)
			if err != nil {
				return fmt.Errorf("load config %q error: %w", cfgPath, err)
			}
			ctx = configs.ContextWithConfig(ctx, cfg, cfgPath)

			// 设置本地化器
			ctx = i18n.ContextWithLocalizer(ctx, i18n.NewLocalizer(globalOpts.Language, cfg.Language, i18n.GetEnvLanguage()))

			keylog, err = setKeyLog()
			if err != nil {
				return fmt.Errorf("set tls key log error: %w", err)
			}

			cmd.SetContext(ctx)

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			series, ok := trading.AllUpdownSeries[args[0]]
			if !ok {
				return fmt.Errorf("series %q not found", args[0])
			}

			var strategy trading.Strategy
			switch opts.Strategy {
			case "discard":
				strategy = trading.DiscardStrategy
			case "monkey":
				strategy = strategies.NewMonkey()
			case "randwalk":
				strategy = strategies.NewRandomWalk()
			default:
				return fmt.Errorf("unknown strategy %q", opts.Strategy)
			}

			trader := trading.NewUpdownSeriesTrader(
				series,
				polymarket.NewClient(polymarket.AuthInfo{}),
				strategy,
				trading.TraderOptions{
					DryRun: opts.DryRun,
					Scale:  opts.Scale,
				},
			)
			ui := tradingui.NewUI(trader)
			return ui.Run(cmd.Context())
		},

		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if keylog != nil {
				_ = keylog.Close()
			}
			return nil
		},
	}

	globalOpts.AddPFlags(cmd.PersistentFlags())
	opts.AddPFlags(cmd.Flags())

	cmd.AddCommand(
		newVersionCommand(),
	)

	return cmd
}

// setKeyLog 设置 TLS keylog
func setKeyLog() (*os.File, error) {
	keylog := os.Getenv("SSLKEYLOGFILE")
	if keylog == "" {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(keylog), 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(keylog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// 设置输出 keylog 文件
	http.DefaultClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			TLSClientConfig: &tls.Config{KeyLogWriter: f},
		},
	}

	return f, nil
}
