package archiver

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/satori/go.uuid"
	"sync"
)

var NAMESPACE_UUID = uuid.FromStringOrNil("b26d2e62-333e-11e6-b557-0cc47a0f7eea")

// This object is a set of instructions for how to create an archivable message
// from some received PayloadObject, though really this should be able to
// operate on any object. Each ArchiveRequest acts as a translator for received
// messages into a single timeseries stream
type ArchiveRequest struct {
	sync.RWMutex
	// AUTOPOPULATED. The entity that requested the URI to be archived.
	FromVK string
	// OPTIONAL. the URI to subscribe to. Requires building a chain on the URI
	// from the .FromVK field. If not provided, uses the base URI of where this
	// ArchiveRequest was stored. For example, if this request was published
	// on <uri>/!meta/archiverequest, then if the URI field was elided it would default
	// to <uri>.
	URI string
	// the URI where this archive request was attached (not populated by user)
	AttachURI string
	// Extracts objects of the given Payload Object type from all messages
	// published on the URI. If elided, operates on all PO types. Can use prefixes, e.g. 2.0.0.0/24
	PO string
	// OPTIONAL. If provided, this is an objectbuilder expr to extract the stream UUID.  If not
	// provided, then a UUIDv3 with NAMESPACE_UUID and the Name field and published URI is generated and used
	UUIDExpr string
	// expression determining how to extract the value from the received
	// message. Expects a number to be returned
	ValueExpr string
	// OPTIONAL. Expression determining how to extract the value from the
	// received message. If not included, it uses the time the message was
	// received on the server.
	TimeExpr string
	// OPTIONAL. Golang time parse string
	TimeParse string
	// Name of the stream. This plus the URI (actual URI, not the URI pattern) is unique
	Name string
	// unit of measure
	Unit string
	// substitution pattern stuff below
	// Perl-style regex for matching a URI (e.g. .*/s.hamilton/(.+?)/i.temperature/signal/operative)
	URIMatch string
	// replacement template. Uses $1 for the first capture group, $2 for the second, etc
	URIReplace string
}

// Print the parameters
func (req *ArchiveRequest) Dump() {
	fmt.Println("┌────────────────")
	fmt.Printf("├ ")
	color.Cyan("ARCHIVE REQUEST")
	fmt.Printf("├ PublishedBy: %s\n", req.FromVK)
	fmt.Printf("├ Archiving: %s\n", req.URI)
	fmt.Printf("├ Name: %s\n", req.Name)
	fmt.Printf("├ Unit: %s\n", req.Unit)
	fmt.Printf("├ PO: %s\n", req.PO)
	fmt.Printf("├ UUID: ")
	if len(req.UUIDExpr) > 0 {
		fmt.Printf("UUID Expression: %s\n", req.UUIDExpr)
	} else {
		fmt.Printf("Autogenerating UUIDs\n")
	}

	fmt.Printf("├ Value Expr: %s\n", req.ValueExpr)
	fmt.Println("├┌")
	if len(req.TimeExpr) > 0 {
		fmt.Printf("│├ Time Expr: %s\n", req.TimeExpr)
		fmt.Printf("│├ Parse Time: %s\n", req.TimeParse)
	} else {
		fmt.Printf("│├ Using server timestamps\n")
	}
	fmt.Println("│└")
	if len(req.URIMatch) > 0 || len(req.URIReplace) > 0 {
		fmt.Printf("├ URI Match: %s\n", req.URIMatch)
		fmt.Printf("├ URI Replace: %s\n", req.URIReplace)
	}
	fmt.Println("└────────────────")
}

// Creates a hash of this object that is unique to its parameters. We will use the URI, PO, UUID and Name
func (req *ArchiveRequest) Hash() string {
	return req.URI + req.Name
}

// Returns true if the two ArchiveRequests are equal
func (req *ArchiveRequest) Equals(other *ArchiveRequest) bool {
	return (req != nil && other != nil) &&
		(req.URI == other.URI) &&
		(req.Name == other.Name) &&
		(req.Unit == other.Unit) &&
		(req.PO == other.PO) &&
		(req.UUIDExpr == other.UUIDExpr) &&
		(req.ValueExpr == other.ValueExpr) &&
		(req.TimeExpr == other.TimeExpr) &&
		(req.TimeParse == other.TimeParse) &&
		(req.URIMatch == other.URIMatch) &&
		(req.URIReplace == other.URIReplace)
}
