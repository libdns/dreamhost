package dreamhost

import (
	"strings"
	"github.com/adamantal/go-dreamhost/api"
	"github.com/libdns/libdns"
)

func (p *Provider) init() error {
	client, err := api.NewClient(p.APIKey, nil)
	if err != nil {
		return err
	}
	p.client = *client
	return nil
}

func recordFromApiDnsRecord(apiDnsRecord api.DNSRecord) libdns.Record {
	return libdns.RR{
		Type: string(apiDnsRecord.Type),
		Data: apiDnsRecord.Value,
		Name: libdns.RelativeName(apiDnsRecord.Record, apiDnsRecord.Zone)
	}.Parse()
}

func apiDnsRecordInputFromRecord(record libdns.Record, zone string) api.DNSRecordInput {
	rr := record.RR()
	var recordInput api.DNSRecordInput
	recordInput.Type = api.RecordType(rr.Type)
	recordInput.Value = rr.Value
	// Dreamhost expects the record name to be absolute, without a dot at the end
	zone = strings.TrimRight(zone, ".")
	recordInput.Record = libdns.AbsoluteName(rr.Name, zone)
	return recordInput
}
