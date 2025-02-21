package db

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/psviderski/uncloud-dns/pkg/model"
	"github.com/psviderski/uncloud-dns/pkg/rand"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	maxSlugHashTimes = 100
	slugLength       = 6
)

type database struct {
	db *gorm.DB
}

// New creates a new database connection
func New(ctx context.Context, engine string, dsn string, config *gorm.Config) (Database, error) {
	if config == nil {
		config = &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		}
	}

	var db *gorm.DB
	var err error

	if engine == "sqlite" {
		db, err = gorm.Open(sqlite.Open(dsn), config)
		db.Exec("PRAGMA foreign_keys = ON")
	} else if engine == "mariadb" {
		db, err = gorm.Open(mysql.Open(dsn), config)
	} else {
		return nil, fmt.Errorf("unsupported dialect: %s", engine)
	}

	db = db.WithContext(ctx)

	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&Domain{},
		&Record{},
	); err != nil {
		return nil, err
	}

	d := &database{
		db: db,
	}
	return d, nil
}

func (d *database) CreateNewSubDomain(tokenHash, domainName string) (Domain, error) {
	var domain Domain
	err := d.db.Transaction(
		func(tx *gorm.DB) error {
			var slug string
			for i := 0; i < maxSlugHashTimes; i++ {
				s := rand.StringWithSmall(slugLength)
				sql := tx.Where("unique_slug = ?", s).Take(&Domain{})
				if sql.Error != nil {
					if sql.Error == gorm.ErrRecordNotFound {
						slug = s
						break
					}
					logrus.Warnf("Error while finding unique slug: %v", sql.Error)
				}
			}
			if slug == "" {
				return fmt.Errorf("couldn't generate slug")
			}
			subDomain := fmt.Sprintf(".%s.%s", slug, domainName)

			domain = Domain{
				TokenHash:   tokenHash,
				UniqueSlug:  slug,
				Domain:      subDomain,
				LastCheckIn: time.Now(),
			}

			sql := tx.Create(&domain)
			if sql.Error != nil {
				return sql.Error
			}

			return nil
		},
	)

	return domain, err
}

func (d *database) GetDomain(domainName string) (Domain, error) {
	domain := Domain{}
	sql := d.db.Where("domain = ?", domainName).Limit(1).Find(&domain)
	return domain, sql.Error
}

func (d *database) Renew(domainID uint, fqdnTypePairs []model.FQDNTypePair, version string) error {

	return d.db.Transaction(
		func(tx *gorm.DB) error {
			now := time.Now()

			// producing separate update queries by type that will look like:
			// update ... where type = 'A' and fqdn in (...) ...
			// update ... where type = 'TXT' and fqdn in (...) ...
			fqdnsByType := make(map[string][]string)
			for _, pair := range fqdnTypePairs {
				fqdns := fqdnsByType[pair.Type]
				fqdns = append(fqdns, pair.FQDN)
				fqdnsByType[pair.Type] = fqdns
			}

			for t, fqdns := range fqdnsByType {
				sql := tx.Model(&Record{}).Where("type = ? and fqdn IN ? and domain_id = ?", t, fqdns, domainID).
					Update("last_check_in", now)
				if sql.Error != nil {
					return sql.Error
				}
			}

			sql := tx.Model(Domain{Model: gorm.Model{ID: domainID}}).Updates(
				map[string]interface{}{"last_check_in": now, "version": version},
			)
			if sql.Error != nil {
				return sql.Error
			}

			return nil
		},
	)
}

func (d *database) PurgeOldDomainsAndRecords(domainMaxAgeSeconds, recordMaxAgeSeconds int64) (int64, int64, error) {
	var domainsDeleted, recordsDeleted int64
	err := d.db.Transaction(
		func(tx *gorm.DB) error {
			lastCheckInDomain := time.Now().Add(-time.Second * time.Duration(domainMaxAgeSeconds))
			lastCheckInRecord := time.Now().Add(-time.Second * time.Duration(recordMaxAgeSeconds))
			sql := d.db.Where("last_check_in < ?", lastCheckInDomain).Delete(&Domain{})
			if sql.Error != nil {
				return sql.Error
			}
			domainsDeleted = sql.RowsAffected

			// domains is a soft delete, so the RowsAffected is accurate. We don't get that for the records' hard delete.
			// Need to get a count first
			sql = d.db.Model(&Record{}).Where("last_check_in < ?", lastCheckInRecord).Count(&recordsDeleted)
			if sql.Error != nil {
				return sql.Error
			}
			sql = d.db.Where("last_check_in < ?", lastCheckInRecord).Delete(&Record{})
			return sql.Error
		},
	)

	return domainsDeleted, recordsDeleted, err
}

func DenormalizeValues(values []string) string {
	sort.Strings(values)
	return strings.Join(values, ",")
}

func (d *database) PersistRecord(domainID uint, fqdn, rType string, values []string) error {
	r, err := d.getRecord(fqdn, rType)
	if err != nil {
		return err
	}

	denormalizeValues := DenormalizeValues(values)

	if r.ID == 0 {
		newRecord := &Record{
			FQDN:        fqdn,
			Type:        rType,
			DomainID:    domainID,
			Values:      denormalizeValues,
			LastCheckIn: time.Now(),
		}
		sql := d.db.Create(newRecord)
		return sql.Error
	}

	r.LastCheckIn = time.Now()
	sql := d.db.Save(r)
	return sql.Error
}

func (d *database) GetYoungRecords(
	maxAgeSeconds int64, fqdnTypePairs map[model.FQDNTypePair]bool,
) (map[model.FQDNTypePair]Record, error) {
	var fqdns []string
	for ftp := range fqdnTypePairs {
		fqdns = append(fqdns, ftp.FQDN)
	}
	lastCheckIn := time.Now().Add(-time.Second * time.Duration(maxAgeSeconds))
	var records []Record
	sql := d.db.Model(&Record{}).Where("fqdn in ? and last_check_in >= ?", fqdns, lastCheckIn).Find(&records)
	if sql.Error != nil {
		return nil, sql.Error
	}

	recordMap := make(map[model.FQDNTypePair]Record)
	for _, r := range records {
		pair := model.FQDNTypePair{FQDN: r.FQDN, Type: r.Type}
		if _, ok := fqdnTypePairs[pair]; ok {
			recordMap[pair] = r
		}
	}
	return recordMap, nil
}

func (d *database) GetDomainRecords(domainID uint) (map[model.FQDNTypePair]Record, error) {
	var records []Record
	sql := d.db.Model(&Record{}).Where("domain_id = ?", domainID).Find(&records)
	if sql.Error != nil {
		return nil, sql.Error
	}

	recordMap := make(map[model.FQDNTypePair]Record)
	for _, r := range records {
		pair := model.FQDNTypePair{FQDN: r.FQDN, Type: r.Type}
		recordMap[pair] = r
	}

	return recordMap, nil
}

func (d *database) GetDomainRecordsByFQDN(fqdn string, domainID uint) ([]Record, error) {
	var records []Record
	sql := d.db.Where("fqdn = ? and domain_id = ?", fqdn, domainID).Find(&records)
	if sql.Error != nil {
		return records, sql.Error
	}

	return records, nil
}

func (d *database) DeleteRecords(records []Record) error {
	sql := d.db.Delete(&records)
	return sql.Error
}

func (d *database) getRecord(fqdn, rType string) (Record, error) {
	record := Record{}
	sql := d.db.Where("fqdn = ? and type = ?", fqdn, rType).Limit(1).Find(&record)
	if sql.Error != nil {
		return record, sql.Error
	}

	return record, nil
}
