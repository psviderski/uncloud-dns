# Uncloud DNS

FQDNs on demand. Powering `xxxxxx.uncld.dev` subdomains.

Will create A, AAAA, CNAME, and TXT records in Route53.

Backed by an SQL database. Supports SQLite for development and MariaDB/MySQL for production.


## CLI

```help
NAME:
   uncloud-dns server - Start API server that manages DNS domains and records in AWS Route53

USAGE:
   uncloud-dns server [command options]

OPTIONS:
   --port value                        HTTP Server Port (default: 4315) [$DNS_PORT]
   --route53-zone-id value             AWS Route53 Zone ID where records will be created [$ROUTE53_ZONE_ID]
   --route53-record-ttl-seconds value  AWS Route53 record TTL (default: 300) [$ROUTE53_RECORD_TTL_SECONDS]
   --purge-interval-seconds value      How often to run the domain and record purge daemon. Default 86,400 (1 day) (default: 86400) [$PURGE_INTERVAL_SECONDS]
   --domain-max-age-seconds value      Max age a domain can be without being renewed before it's deleted. Default 2,592,000 (30 days) (default: 2592000) [$DOMAIN_MAX_AGE_SECONDS]
   --record-max-age-seconds value      Max age a domain can be without being renewed before it's deleted. Default 172,800 (2 days) (default: 172800) [$RECORD_MAX_AGE_SECONDS]
   --db-engine value                   The type of DB to connect to, sqlite or mariadb (default: "sqlite") [$DB_ENGINE]
   --db-sqlite-dsn value               The DSN to use to connect to a sqlite db (default: "file:db.sqlite?_pragma=foreign_keys(1)") [$DB_SQLITE_DSN]
   --db-user value                     Database user [$DB_USER]
   --db-password value                 Database password [$DB_PASSWORD]
   --db-name value                     Name of the database [$DB_NAME]
   --db-host value                     Database host [$DB_HOST]
   --db-port value                     Database port [$DB_PORT]
   --log-level value, -l value         Log Level (default: "info") [$LOGLEVEL]
   --log-caller                        log the caller (aka line number and file) (default: false)
   --help, -h                          show help
```
