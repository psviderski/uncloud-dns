package backend

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/psviderski/uncloud-dns/pkg/db"
	"github.com/psviderski/uncloud-dns/pkg/model"
	"github.com/psviderski/uncloud-dns/pkg/rand"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/exp/maps"
)

const (
	tokenLength = 32
)

type backend struct {
	baseDomain           string
	ZoneID               string
	legacyBaseDomain     string
	legacyZoneID         string
	recordTTLSeconds     int64
	purgeIntervalSeconds int64
	domainMaxAgeSeconds  int64
	recordMaxAgeSeconds  int64

	Svc *route53.Route53
	db  db.Database
}

func NewBackend(
	zoneID, legacyZoneID string, recordTTLSecs, purgeIntervalSecs, domainMaxAgeSecs, recordMaxAgeSecs int64,
	database db.Database,
) (Backend, error) {
	s, err := session.NewSession()
	if err != nil {
		return &backend{}, err
	}

	svc := route53.New(
		s, &aws.Config{
			MaxRetries: aws.Int(3),
		},
	)

	z, err := svc.GetHostedZone(
		&route53.GetHostedZoneInput{
			Id: aws.String(zoneID),
		},
	)
	if err != nil {
		return &backend{}, err
	}

	// Initialise the legacy zone if provided.
	var legacyBaseDomain, normalizedLegacyZoneID string
	if legacyZoneID != "" {
		lz, err := svc.GetHostedZone(
			&route53.GetHostedZoneInput{
				Id: aws.String(legacyZoneID),
			},
		)
		if err != nil {
			return &backend{}, err
		}
		legacyBaseDomain = strings.TrimSuffix(aws.StringValue(lz.HostedZone.Name), ".")
		normalizedLegacyZoneID = aws.StringValue(lz.HostedZone.Id)
	}

	return &backend{
		db:                   database,
		baseDomain:           strings.TrimSuffix(aws.StringValue(z.HostedZone.Name), "."),
		ZoneID:               aws.StringValue(z.HostedZone.Id),
		legacyBaseDomain:     legacyBaseDomain,
		legacyZoneID:         normalizedLegacyZoneID,
		Svc:                  svc,
		recordTTLSeconds:     recordTTLSecs,
		purgeIntervalSeconds: purgeIntervalSecs,
		domainMaxAgeSeconds:  domainMaxAgeSecs,
		recordMaxAgeSeconds:  recordMaxAgeSecs,
	}, nil
}

// zoneForDomain returns the zone ID for operations on the given domain.
// If the domain belongs to the legacy zone, it returns the legacy zone ID.
func (b *backend) zoneForDomain(domain string) string {
	if b.legacyZoneID != "" && strings.HasSuffix(domain, b.legacyBaseDomain) {
		return b.legacyZoneID
	}
	return b.ZoneID
}

func (b *backend) GetDomain(domainName string) (db.Domain, error) {
	logrus.Debugf("get record for domain: %v", domainName)
	return b.db.GetDomain(domainName)
}

func (b *backend) Renew(
	domain string, domainID uint, records []model.RecordRequest, version string,
) ([]model.FQDNTypePair, error) {
	recordMap := make(map[model.FQDNTypePair]model.RecordRequest)
	var cleanedRecords []model.FQDNTypePair
	// remove duplicates and FQDNs that don't belong to this domain
	for _, record := range records {
		fqdn := record.Name
		// The renew request sends just the "short" name of the FQDN, not the full thing, so we need to construct it.
		// Checking for and handling both forms (as well as a present or absent "." prefixing the domain) is just being
		// overly cautious/forgiving of the request
		if !strings.HasSuffix(record.Name, domain) {
			if strings.HasPrefix(domain, ".") {
				fqdn = record.Name + domain
			} else {
				fqdn = record.Name + "." + domain
			}
		} else if strings.HasSuffix(record.Name, b.baseDomain) {
			// Record is an FQDN, but doesn't match the validated domain. Error out.
			return nil, fmt.Errorf("invalid record %v doesn't match %v", record.Name, domain)
		}
		pair := model.FQDNTypePair{
			FQDN: fqdn,
			Type: record.Type,
		}
		if _, ok := recordMap[pair]; !ok {
			cleanedRecords = append(cleanedRecords, pair)
		}
		recordMap[pair] = record
	}

	if err := b.db.Renew(domainID, cleanedRecords, version); err != nil {
		return nil, err
	}

	domainRecords, err := b.db.GetDomainRecords(domainID)
	if err != nil {
		return nil, err
	}

	var outOfSync []model.FQDNTypePair
	for pair, record := range recordMap {
		if dr, ok := domainRecords[pair]; ok {
			if dr.Values != db.DenormalizeValues(record.Values) {
				// The renew request should return the "short" name part of the FQDN, not the entire FQDN
				outOfSync = append(outOfSync, model.FQDNTypePair{FQDN: record.Name, Type: record.Type})
			}
		} else {
			outOfSync = append(outOfSync, model.FQDNTypePair{FQDN: record.Name, Type: record.Type})
		}
	}

	return outOfSync, nil
}

func (b *backend) CreateDomain() (model.DomainResponse, error) {
	logrus.Debugf("Creating a new domain")
	token, hash, err := b.createToken()
	if err != nil {
		return model.DomainResponse{}, err
	}

	domain, err := b.db.CreateNewSubDomain(hash, b.baseDomain)
	if err != nil {
		return model.DomainResponse{}, err
	}

	return model.DomainResponse{
		Name:  domain.Domain,
		Token: token,
	}, nil
}

func (b *backend) DeleteRecord(recordPrefix string, domain string, domainID uint) error {
	fqdn := recordPrefix + domain

	records, err := b.db.GetDomainRecordsByFQDN(fqdn, domainID)
	if err != nil {
		return err
	}

	if err = b.doRecordsDelete(records); err != nil {
		return fmt.Errorf("failed to delete route53 records for FQDN %v with error %v", fqdn, err)
	}
	return nil
}

func (b *backend) PurgeRecords(domain string, domainID uint) error {
	recs, err := b.db.GetDomainRecords(domainID)
	if err != nil {
		return err
	}
	records := maps.Values(recs)
	if err = b.doRecordsDelete(records); err != nil {
		return fmt.Errorf("failed to delete route53 records for domain %v with error %v", domain, err)
	}
	return nil
}

func (b *backend) doRecordsDelete(records []db.Record) error {
	if len(records) == 0 {
		return nil
	}

	// Group records by zone based on their FQDN.
	changesByZone := make(map[string][]*route53.Change)
	for _, record := range records {
		rrs := &route53.ResourceRecordSet{
			Type: aws.String(record.Type),
			Name: aws.String(record.FQDN),
			TTL:  aws.Int64(b.recordTTLSeconds),
		}
		rr := make([]*route53.ResourceRecord, 0)
		for _, value := range strings.Split(record.Values, ",") {
			rr = append(
				rr, &route53.ResourceRecord{
					Value: aws.String(cleanRecordValue(record.Type, value)),
				},
			)
		}
		rrs.ResourceRecords = rr

		zoneID := b.zoneForDomain(record.FQDN)
		changesByZone[zoneID] = append(changesByZone[zoneID], &route53.Change{
			Action:            aws.String("DELETE"),
			ResourceRecordSet: rrs,
		})
	}

	// Delete records from each zone.
	for zoneID, changes := range changesByZone {
		rrsInput := route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &route53.ChangeBatch{
				Changes: changes,
			},
		}

		if _, err := b.Svc.ChangeResourceRecordSets(&rrsInput); err != nil {
			return err
		}
	}

	return b.db.DeleteRecords(records)
}

func (b *backend) CreateRecord(domain string, domainID uint, input model.RecordRequest) (model.RecordResponse, error) {
	rr := make([]*route53.ResourceRecord, 0)

	for _, value := range input.Values {
		rr = append(
			rr, &route53.ResourceRecord{
				Value: aws.String(cleanRecordValue(input.Type, value)),
			},
		)
	}

	fqdn := fmt.Sprintf("%s.%s", input.Name, domain)
	rrs := &route53.ResourceRecordSet{
		Type:            aws.String(input.Type),
		Name:            aws.String(fqdn),
		ResourceRecords: rr,
		TTL:             aws.Int64(b.recordTTLSeconds),
	}

	rrsInput := route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(b.zoneForDomain(domain)),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action:            aws.String("UPSERT"),
					ResourceRecordSet: rrs,
				},
			},
		},
	}

	if _, err := b.Svc.ChangeResourceRecordSets(&rrsInput); err != nil {
		return model.RecordResponse{}, fmt.Errorf("failed to upsert route53 record %v with error %v", fqdn, err)
	}

	if err := b.db.PersistRecord(domainID, fqdn, input.Type, input.Values); err != nil {
		return model.RecordResponse{}, err
	}

	return model.RecordResponse{
		RecordRequest: input,
		FQDN:          fqdn,
	}, nil
}

func cleanRecordValue(rType string, value string) string {
	if rType == model.RecordTypeTxt && !strings.HasPrefix(value, "\"") {
		return "\"" + value + "\""
	}

	return value
}

func (b *backend) createToken() (string, string, error) {
	t := rand.StringWithAll(tokenLength)
	hash, err := bcrypt.GenerateFromPassword([]byte(t), bcrypt.MinCost)
	if err != nil {
		return "", "", err
	}
	return t, string(hash), nil
}
