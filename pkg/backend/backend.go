package backend

import (
	"github.com/psviderski/uncloud-dns/pkg/db"
	"github.com/psviderski/uncloud-dns/pkg/model"
)

type Backend interface {
	GetDomain(domainName string) (db.Domain, error)
	CreateDomain() (model.DomainResponse, error)
	Renew(domain string, domainID uint, records []model.RecordRequest, version string) ([]model.FQDNTypePair, error)
	PurgeRecords(domain string, domainID uint) error
	CreateRecord(domain string, domainID uint, input model.RecordRequest) (model.RecordResponse, error)
	DeleteRecord(recordPrefix string, domain string, domainID uint) error
	StartPurgerDaemon(done <-chan struct{})
}
