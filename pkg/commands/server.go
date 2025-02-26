package commands

import (
	"fmt"

	"github.com/psviderski/uncloud-dns/pkg/apiserver"
	"github.com/psviderski/uncloud-dns/pkg/backend"
	"github.com/psviderski/uncloud-dns/pkg/db"
	"github.com/psviderski/uncloud-dns/pkg/version"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

type apiServerCommand struct{}

func (s *apiServerCommand) Execute(c *cli.Context) error {
	ctx := signals.SetupSignalContext()

	log := logrus.WithField("command", "server")

	log.Infof("version: %v", version.Get())

	engine, dsn, err := constructDSN(c)
	if err != nil {
		return err
	}

	database, err := db.New(
		ctx, engine, dsn,
		&gorm.Config{Logger: db.NewLogger(c.String("log-level"))},
	)
	if err != nil {
		return err
	}

	back, err := backend.NewBackend(
		c.String("route53-zone-id"),
		c.Int64("route53-record-ttl-seconds"),
		c.Int64("purge-interval-seconds"),
		c.Int64("domain-max-age-seconds"),
		c.Int64("record-max-age-seconds"),
		database,
	)
	if err != nil {
		return err
	}

	apiServer := apiserver.NewAPIServer(ctx, log, c.Int("port"))

	if err := apiServer.Start(back); err != nil {
		return err
	}

	return nil
}

func constructDSN(c *cli.Context) (string, string, error) {
	engine := c.String("db-engine")
	if engine == "sqlite" {
		return engine, c.String("db-sqlite-dsn"), nil
	} else if engine == "mariadb" {
		user := c.String("db-user")
		if user == "" {
			return "", "", fmt.Errorf("missing database user")
		}
		password := c.String("db-password")
		if password == "" {
			return "", "", fmt.Errorf("missing database password")
		}
		name := c.String("db-name")
		if name == "" {
			return "", "", fmt.Errorf("missing database name")
		}
		host := c.String("db-host")
		if host == "" {
			return "", "", fmt.Errorf("missing database host")
		}
		port := c.String("db-port")
		if port == "" {
			return "", "", fmt.Errorf("missing database port")
		}

		dsn := fmt.Sprintf(
			"%v:%v@tcp(%v:%v)/%v?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, name,
		)
		return engine, dsn, nil
	} else {
		return "", "", fmt.Errorf("unsupported db engine: %v", engine)
	}
}

func serverCommand() *cli.Command {
	cmd := apiServerCommand{}

	flags := []cli.Flag{
		&cli.IntFlag{
			Name:    "port",
			Usage:   "HTTP Server Port",
			EnvVars: []string{"DNS_PORT"},
			Value:   4315,
		},
		&cli.StringFlag{
			Name:     "route53-zone-id",
			Usage:    "AWS Route53 Zone ID where records will be created",
			EnvVars:  []string{"ROUTE53_ZONE_ID"},
			Required: true,
		},
		&cli.Int64Flag{
			Name:    "route53-record-ttl-seconds",
			Usage:   "AWS Route53 record TTL",
			EnvVars: []string{"ROUTE53_RECORD_TTL_SECONDS"},
			Value:   300,
		},
		&cli.Int64Flag{
			Name:    "purge-interval-seconds",
			Usage:   "How often to run the domain and record purge daemon. Set to 0 to disable. Default 86,400 (1 day)",
			EnvVars: []string{"PURGE_INTERVAL_SECONDS"},
			Value:   86400,
		},
		&cli.Int64Flag{
			Name:    "domain-max-age-seconds",
			Usage:   "Max age a domain can be without being renewed before it's deleted. Default 2,592,000 (30 days)",
			EnvVars: []string{"DOMAIN_MAX_AGE_SECONDS"},
			Value:   2592000,
		},
		&cli.Int64Flag{
			Name:    "record-max-age-seconds",
			Usage:   "Max age a domain can be without being renewed before it's deleted. Default 172,800 (2 days)",
			EnvVars: []string{"RECORD_MAX_AGE_SECONDS"},
			Value:   172800,
		},
		&cli.StringFlag{
			Name:    "db-engine",
			Usage:   "The type of DB to connect to, sqlite or mariadb",
			EnvVars: []string{"DB_ENGINE"},
			Value:   "sqlite",
		},
		&cli.StringFlag{
			Name:    "db-sqlite-dsn",
			Usage:   "The DSN to use to connect to a sqlite db",
			EnvVars: []string{"DB_SQLITE_DSN"},
			Value:   "file:db.sqlite?_pragma=foreign_keys(1)",
		},
		&cli.StringFlag{
			Name:    "db-user",
			Usage:   "Database user",
			EnvVars: []string{"DB_USER"},
		},
		&cli.StringFlag{
			Name:    "db-password",
			Usage:   "Database password",
			EnvVars: []string{"DB_PASSWORD"},
		},
		&cli.StringFlag{
			Name:    "db-name",
			Usage:   "Name of the database",
			EnvVars: []string{"DB_NAME"},
		},
		&cli.StringFlag{
			Name:    "db-host",
			Usage:   "Database host",
			EnvVars: []string{"DB_HOST"},
		},
		&cli.StringFlag{
			Name:    "db-port",
			Usage:   "Database port",
			EnvVars: []string{"DB_PORT"},
		},
	}

	return &cli.Command{
		Name:   "server",
		Usage:  "Start API server that manages DNS domains and records in AWS Route53",
		Action: cmd.Execute,
		Flags:  append(flags, GlobalFlags()...),
		Before: Before,
	}
}
