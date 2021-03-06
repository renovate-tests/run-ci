package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/suzuki-shunsuke/go-ci-env/cienv"
	"github.com/suzuki-shunsuke/run-ci/pkg/config"
	"github.com/suzuki-shunsuke/run-ci/pkg/constant"
	"github.com/suzuki-shunsuke/run-ci/pkg/controller"
	"github.com/suzuki-shunsuke/run-ci/pkg/execute"
	"github.com/suzuki-shunsuke/run-ci/pkg/expr"
	"github.com/suzuki-shunsuke/run-ci/pkg/git"
	"github.com/suzuki-shunsuke/run-ci/pkg/github"
	"github.com/urfave/cli/v2"
)

type Runner struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func (runner Runner) Run(ctx context.Context, args ...string) error {
	app := cli.App{
		Name:    "run-ci",
		Usage:   "run CI automatically when pull request's base branch is updated. https://github.com/suzuki-shunsuke/run-ci",
		Version: constant.Version,
		Commands: []*cli.Command{
			{
				Name:   "update-pr",
				Usage:  "run pull requests' CI",
				Action: runner.action,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "owner",
						Usage: "repository owner",
					},
					&cli.StringFlag{
						Name:  "repo",
						Usage: "repository name",
					},
					&cli.StringFlag{
						Name:  "github-token",
						Usage: "GitHub Access Token [$GITHUB_TOKEN, $GITHUB_ACCESS_TOKEN]",
					},
					&cli.StringFlag{
						Name:  "base",
						Usage: "base branch. Either the option 'base' or 'all' should be set",
					},
					&cli.BoolFlag{
						Name:  "all",
						Usage: "get pull requests without specifying the base branch. Either the option 'base' or 'all' should be set",
					},
					&cli.StringFlag{
						Name:  "log-level",
						Usage: "log level",
					},
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "configuration file path",
					},
				},
			},
			{
				Name:   "init",
				Usage:  "generate a configuration file if it doesn't exist",
				Action: runner.initAction,
			},
		},
	}

	return app.RunContext(ctx, args)
}

var (
	ErrGitHubAccessTokenIsRequired error = errors.New("GitHub Access Token is required")
	ErrOwnerIsRequired             error = errors.New("owner is required")
	ErrRepoIsRequired              error = errors.New("repo is required")
	ErrEitherAllorBaseIsRequired   error = errors.New("either the option 'base' or 'all' should be set")
	ErrBothAllAndBaseCantBeSet     error = errors.New("both the option 'base' and 'all' can't be set at the same time")
)

func (runner Runner) setCLIArg(c *cli.Context, cfg config.Config) config.Config {
	if owner := c.String("owner"); owner != "" {
		cfg.Owner = owner
	}
	if repo := c.String("repo"); repo != "" {
		cfg.Repo = repo
	}
	if token := c.String("github-token"); token != "" {
		cfg.GitHubToken = token
	}
	if base := c.String("base"); base != "" {
		cfg.Base = base
	}
	if c.Bool("all") {
		cfg.All = true
	}
	if logLevel := c.String("log-level"); logLevel != "" {
		cfg.LogLevel = logLevel
	}
	return cfg
}

func (runner Runner) readConfig(c *cli.Context) (config.Config, error) {
	reader := config.Reader{
		ExistFile: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
	}

	cfgPath := c.String("config")

	wd, err := os.Getwd()
	if err != nil {
		return config.Config{}, err
	}

	return reader.FindAndRead(cfgPath, wd)
}

func (runner Runner) action(c *cli.Context) error { //nolint:funlen
	cfg, err := runner.readConfig(c)
	if err != nil {
		return err
	}

	cfg = runner.setCLIArg(c, cfg)
	cfg = config.SetEnv(cfg)
	cfg = config.SetDefault(cfg)

	// platform environment variables
	platform := cienv.Get()
	if platform != nil {
		if cfg.Owner == "" {
			cfg.Owner = platform.RepoOwner()
		}
		if cfg.Repo == "" {
			cfg.Repo = platform.RepoName()
		}
	}

	// validation
	if cfg.Owner == "" {
		return ErrOwnerIsRequired
	}
	if cfg.Repo == "" {
		return ErrRepoIsRequired
	}

	if !cfg.All && cfg.Base == "" {
		return ErrEitherAllorBaseIsRequired
	}
	if cfg.All && cfg.Base != "" {
		return ErrBothAllAndBaseCantBeSet
	}

	ghClient := github.New(c.Context, github.ParamsNew{
		Token: cfg.GitHubToken,
	})

	if cfg.LogLevel != "" {
		lvl, err := logrus.ParseLevel(cfg.LogLevel)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"log_level": cfg.LogLevel,
			}).WithError(err).Error("the log level is invalid")
		}
		logrus.SetLevel(lvl)
	}

	logrus.WithFields(logrus.Fields{
		"expr":             cfg.Expr,
		"owner":            cfg.Owner,
		"repo":             cfg.Repo,
		"empty_commit_msg": cfg.EmptyCommitMsg,
		"base":             cfg.Base,
		"all":              cfg.All,
		"log_level":        cfg.LogLevel,
	}).Debug("config")
	ex, err := expr.New(cfg.Expr)
	if err != nil {
		return fmt.Errorf("it is failed to compile the expression. Please check the expression: %w", err)
	}

	ctrl := controller.Controller{
		Config: cfg,
		GitHub: ghClient,
		Expr:   ex,
		Git: git.New(git.ParamsNew{
			UserName:  cfg.GitCommand.UserName,
			UserEmail: cfg.GitCommand.UserEmail,
			Executor:  execute.New(),
		}),
	}

	return ctrl.UpdatePR(c.Context)
}
