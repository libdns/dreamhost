package dreamhost

import (
	"context"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/adamantal/go-dreamhost/api"
	"github.com/libdns/libdns"
)

// Provider facilitates DNS record manipulation with Dreamhost.
type Provider struct {
	APIKey string `json:"api_key,omitempty"`
	Logger *zap.Logger
	client api.Client
	once   sync.Once
	mutex  sync.Mutex
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	err := p.init()
	if err != nil {
		return nil, err
	}

	var records []libdns.Record

	apiRecords, err := p.client.ListDNSRecords(ctx)
	if err != nil {
		return nil, err
	}

	// translate each Dreamhost Domain Record to a libdns Record
	for _, rec := range apiRecords {
		if rec.Zone == zone {
			records = append(records, recordFromApiDnsRecord(rec))
		}

	}

	return records, nil
}

func (p *Provider) addDNSRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var createdRecords []libdns.Record
	p.getLogger().Info("[dreamhost] Creating DNS records", zap.String("zone", zone), zap.Any("records", records))
	for _, record := range records {
		apiInputRecord := apiDnsRecordInputFromRecord(record, zone)
		err := p.client.AddDNSRecord(ctx, apiInputRecord)
		if err == nil {
			createdRecords = append(createdRecords, record)
		}
	}

	return createdRecords, nil
}

func (p *Provider) removeDNSRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var deletedRecords []libdns.Record

	for _, record := range records {
		apiInputRecord := apiDnsRecordInputFromRecord(record, zone)
		err := p.client.RemoveDNSRecord(ctx, apiInputRecord)
		if err == nil {
			deletedRecords = append(deletedRecords, record)
		}
	}

	return deletedRecords, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	err := p.init()
	if err != nil {
		return nil, err
	}

	created, err := p.addDNSRecords(ctx, zone, records)
	if err != nil {
		return nil, err
	}

	// Verify each TXT record has been propagated before returning success
	// Dreamhost seems to take a particularly long time to propagate new TXT records
	for _, record := range created {
		rr := record.RR()
		if rr.Type == "TXT" {
			fqdn := rr.Name + "." + zone
			err := p.waitForPropagation(ctx, fqdn, rr.Data)
			if err != nil {
				return nil, err
			}
		}
	}
	return created, nil
}

// SetRecords sets the records in the zone, either by updating existing records or creating new ones.
// It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	err := p.init()
	if err != nil {
		return nil, err
	}

	return p.addDNSRecords(ctx, zone, records)
}

// DeleteRecords deletes the records from the zone. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	err := p.init()
	if err != nil {
		return nil, err
	}

	return p.removeDNSRecords(ctx, zone, records)
}

func (p *Provider) waitForPropagation(ctx context.Context, fqdn string, expectedValue string) error {
	p.getLogger().Info("[dreamhost] waiting for TXT record to propagate...", zap.String("fqdn",fqdn))
	for {
		records, err := net.DefaultResolver.LookupTXT(ctx, fqdn)
		if err == nil {
			for _, r := range records {
				if r == expectedValue {
					p.getLogger().Info("[dreamhost] TXT record found, propagation verified")
					return nil
				}
			}
			p.getLogger().Info("[dreamhost] TXT record not yet visible, retrying in 15s")
		} else {
			p.getLogger().Info("[dreamhost] lookup error, retrying in 15s", zap.String("error",err.Error()))
		}
		select{
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(15 * time.Second):
		}
	}
}

func (p *Provider) getLogger() *zap.Logger {
	if p.Logger == nil {
		p.Logger = zap.Must(zap.NewProduction())
	}
	return p.Logger
}

// Interface guards
var (
	_ libdns.RecordGetter	= (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter	= (*Provider)(nil)
	_ libdns.RecordDeleter	= (*Provider)(nil)
)
