package db

import (
	"github.com/psviderski/uncloud-dns/pkg/model"
)

type Database interface {
	CreateNewSubDomain(tokenHash, domainName string) (Domain, error)
	GetDomain(domain string) (Domain, error)
	PersistRecord(domainID uint, fqdn, rType string, values []string) error
	Renew(domainID uint, fqdnTypePairs []model.FQDNTypePair, version string) error
	GetDomainRecords(domainID uint) (map[model.FQDNTypePair]Record, error)
	GetDomainRecordsByFQDN(fqdn string, domainID uint) ([]Record, error)
	DeleteRecords(records []Record) error
	PurgeOldDomainsAndRecords(maxDomainAgeSeconds, maxRecordAgeSeconds int64) (int64, int64, error)
	GetYoungRecords(maxAgeSeconds int64, fqdnTypePairs map[model.FQDNTypePair]bool) (
		map[model.FQDNTypePair]Record, error,
	)
}
