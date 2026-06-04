package parser

// PacketType is a bitmask of the high-level packet category, used by type
// filters (t/...).
type PacketType uint32

const (
	TypePosition   PacketType = 1 << iota // p - position (compressed/uncompressed/mic-e)
	TypeObject                            // o - object
	TypeItem                              // i - item
	TypeMessage                           // m - message (incl. ack/rej)
	TypeQuery                             // q - query
	TypeStatus                            // s - status report
	TypeTelemetry                         // t - telemetry
	TypeUserDef                           // u - user-defined
	TypeWeather                           // w - weather
	TypeNWS                               // n - NWS broadcast (subset of message/object)
	TypeBulletin                          // bulletin / announcement (subset of message)
	TypeThirdParty                        // third-party traffic wrapper
	TypeNMEA                              // raw NMEA / GPS sentence
	TypeCWOP                              // c - CWOP weather (subset of weather)
)

// Has reports whether the given type bit is set.
func (t PacketType) Has(b PacketType) bool { return t&b != 0 }

// Parsed is a struct that storage parsed APRS packet
type Parsed struct {
	Raw            string
	From           string
	To             string
	Path           []string
	Format         string
	PacketType     PacketType
	HasPosition    bool
	Symbol         []string
	Lat            float64
	Lon            float64
	Comment        string
	MessageCapable bool
	ObjectName     string
	ObjectFormat   string
	Alive          bool
	RawTimestamp   string
	Timestamp      int
	GPSFixStatus   bool
	Altitude       float64
	Course         float64
	Speed          float64
	RadioRange     float64
	PosAmbiguity   int
	Bearing        int
	Title          string
	NRQ            int
	PHG            string
	PHGPower       float64
	PHGHeight      float64
	PHGGain        float64
	PHGDir         string
	PHGRange       float64
	PHGRate        int
	RNG            float64
	DAODatumByte   string
	Telemetry      TelemetryData
	TelemetryMicE  []int
	TPARM          []string
	TUNIT          []string
	TEQNS          [][]float64
	TBITS          string
	Weather        map[string]float64
	SubPacket      *Parsed
	Body           string
	ID             string
	Type           string
	Status         string
	MessageText    string
	AID            string
	BID            string
	Identifier     string
	Addressee      string
	Response       string
	MsgNo          string
	AckMsgNo       string
	MType          string
	MBits          string
}
