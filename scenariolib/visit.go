package scenariolib

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	ua "github.com/coveo/go-coveo/analytics"
	"github.com/coveo/go-coveo/search"
	"github.com/k0kubun/pp"
)

// Visit        The struct visit is used to store one visit to the site.
// SearchClient The http client to send search queries
// UAClient     The http client to send the usage analytics data
// LastQuery    The last query that was searched
// LastResponse The last response that was received
// Username     The name of the user visiting
// OriginLevel1 Where the events originate from
// OriginLevel2 Same as OriginLevel1
// LastTab      The tab the user last visited
type Visit struct {
	SearchClient search.Client
	UAClient     ua.Client
	LastQuery    *search.Query
	LastResponse *search.Response
	Username     string
	OriginLevel1 string
	OriginLevel2 string
	LastTab      string
}

const (
	// JSUIVERSION Change this to the version of JSUI you want to appear to be using.
	JSUIVERSION string = "0.0.0.0;0.0.0.0"
	// TIMEBETWEENACTIONS The time in seconds to wait between the different actions inside a visit
	TIMEBETWEENACTIONS int = 5
)

// NewVisit     Creates a new visit to the search page
// _searchtoken The token used to be able to search
// _uatoken     The token used to send usage analytics events
// _useragent   The user agent the analytics events will see
func NewVisit(_searchtoken string, _uatoken string, _useragent string, c *Config) (*Visit, error) {
	v := Visit{}
	v.Username = buildUserEmail(c)
	pp.Printf("\n\nLOG >>> New visit from %v\n", v.Username)
	pp.Printf("LOG >>> On device %v\n", _useragent)

	// Create the http searchClient
	searchConfig := search.Config{Token: _searchtoken, UserAgent: _useragent, Endpoint: c.SearchEndpoint}
	searchClient, err := search.NewClient(searchConfig)
	if err != nil {
		return nil, err
	}
	v.SearchClient = searchClient

	// Create the http UAClient
	uaConfig := ua.Config{Token: _uatoken, UserAgent: _useragent, IP: c.RandomIPs[rand.Intn(len(c.RandomIPs))], Endpoint: c.AnalyticsEndpoint}
	uaClient, err := ua.NewClient(uaConfig)
	if err != nil {
		return nil, err
	}
	v.UAClient = uaClient

	return &v, nil
}

func buildUserEmail(c *Config) string {
	return fmt.Sprint(c.FirstNames[rand.Intn(len(c.FirstNames))], ".", c.LastNames[rand.Intn(len(c.LastNames))], c.Emails[rand.Intn(len(c.Emails))])
}

// ExecuteScenario Execute a specific scenario, send the config for all the
// potential random we need to do.
func (v *Visit) ExecuteScenario(scenario Scenario, c *Config) error {
	pp.Printf("\nLOG >>> Executing scenario named : %v", scenario.Name)
	for i := 0; i < len(scenario.Events); i++ {
		jsonEvent := scenario.Events[i]
		event, err := ParseEvent(&jsonEvent, c)
		if err != nil {
			return err
		}

		err = event.Execute(v)
		if err != nil {
			return err
		}

		WaitBetweenActions()
	}
	return nil
}

func (v *Visit) sendSearchEvent(q string) error {
	pp.Printf("\nLOG >>> Sending Search Event : %v results", v.LastResponse.TotalCount)
	se, err := ua.NewSearchEvent()
	if err != nil {
		return err
	}

	se.Username = v.Username
	se.SearchQueryUID = v.LastResponse.SearchUID
	se.QueryText = q
	se.AdvancedQuery = v.LastQuery.AQ
	se.ActionCause = "searchboxSubmit"
	se.OriginLevel1 = v.OriginLevel1
	se.OriginLevel2 = v.OriginLevel2
	se.NumberOfResults = v.LastResponse.TotalCount
	se.ResponseTime = v.LastResponse.Duration
	se.CustomData = map[string]interface{}{
		"JSUIVersion": JSUIVERSION,
	}

	if v.LastResponse.TotalCount > 0 {
		if urihash, ok := v.LastResponse.Results[0].Raw["sysurihash"].(string); ok {
			se.Results = []ua.ResultHash{
				ua.ResultHash{DocumentURI: v.LastResponse.Results[0].URI, DocumentURIHash: urihash},
			}
		} else {
			return errors.New("ERR >>> Cannot convert sysurihash to string in search event")
		}
	}

	// Send a UA search event
	error := v.UAClient.SendSearchEvent(se)
	if error != nil {
		return err
	}
	return nil
}

func (v *Visit) sendViewEvent(pageTitle, pageReferrer, pageURI string) error {
	pp.Printf("\nLOG >>> Sending PageView Event on URI: %v\n", pageURI)

	ve := ua.NewViewEvent()

	ve.Username = v.Username
	ve.OriginLevel1 = v.OriginLevel1
	ve.OriginLevel2 = v.OriginLevel2
	ve.Anonymous = false
	ve.PageReferrer = pageReferrer
	ve.PageTitle = pageTitle
	ve.PageURI = pageURI
	ve.CustomData = map[string]interface{}{
		"JSUIVersion": JSUIVERSION,
	}

	// Send a UA search event
	err := v.UAClient.SendViewEvent(ve)
	return err
}

func (v *Visit) sendCustomEvent(eventType string, eventValue string) error {
	pp.Printf("\nLOG >>> Sending Custom Event type=%v && value=%v", eventType, eventValue)
	ce, err := ua.NewCustomEvent()
	if err != nil {
		return err
	}

	ce.Username = v.Username
	//ce.LastSearchQueryUID = v.LastResponse.SearchUID
	//ce.EventType = eventType
	//ce.EventValue = eventValue
	ce.OriginLevel1 = v.OriginLevel1
	ce.OriginLevel2 = v.OriginLevel2
	ce.CustomData = map[string]interface{}{
		"JSUIVersion": JSUIVERSION,
	}

	// Send a UA search event
	err = v.UAClient.SendCustomEvent(ce)
	return err
}

func (v *Visit) SendClickEvent(rank int, quickview bool) error {
	event, err := ua.NewClickEvent()
	if err != nil {
		return err
	}

	event.DocumentURI = v.LastResponse.Results[rank].URI
	event.SearchQueryUID = v.LastResponse.SearchUID
	event.DocumentPosition = rank + 1
	if quickview {
		event.ActionCause = "documentQuickview"
		event.ViewMethod = "documentQuickview"
	} else {
		event.ActionCause = "documentOpen"
	}

	event.DocumentTitle = v.LastResponse.Results[rank].Title
	event.QueryPipeline = v.LastResponse.Pipeline
	event.DocumentURL = v.LastResponse.Results[rank].ClickUri
	event.Username = v.Username
	event.OriginLevel1 = v.OriginLevel1
	event.OriginLevel2 = v.OriginLevel2
	if urihash, ok := v.LastResponse.Results[rank].Raw["sysurihash"].(string); ok {
		event.DocumentURIHash = urihash
	} else {
		return errors.New("ERR >>> Cannot convert sysurihash to string")
	}
	if collection, ok := v.LastResponse.Results[rank].Raw["syscollection"].(string); ok {
		event.CollectionName = collection
	} else {
		return errors.New("ERR >>> Cannot convert syscollection to string")
	}
	if source, ok := v.LastResponse.Results[rank].Raw["syssource"].(string); ok {
		event.SourceName = source
	} else {
		return errors.New("ERR >>> Cannot convert syssource to string")
	}

	err = v.UAClient.SendClickEvent(event)
	if err != nil {
		return err
	}
	return nil
}

func (v *Visit) sendInterfaceChangeEvent(actionCause, actionType string, customData map[string]interface{}) error {
	ice, err := ua.NewSearchEvent()
	if err != nil {
		return err
	}

	ice.Username = v.Username
	ice.SearchQueryUID = v.LastResponse.SearchUID
	ice.QueryText = v.LastQuery.Q
	ice.AdvancedQuery = v.LastQuery.AQ
	//ice.ActionCause = "interfaceChange"
	ice.ActionCause = actionCause
	ice.ActionType = actionType
	ice.OriginLevel1 = v.OriginLevel1
	ice.OriginLevel2 = v.OriginLevel2
	ice.NumberOfResults = v.LastResponse.TotalCount
	ice.ResponseTime = v.LastResponse.Duration
	ice.CustomData = customData
	/*ice.CustomData = map[string]interface{}{
		"interfaceChangeTo": v.OriginLevel2,
		"JSUIVersion":       JSUIVERSION,
	}*/

	if v.LastResponse.TotalCount > 0 {
		if urihash, ok := v.LastResponse.Results[0].Raw["sysurihash"].(string); ok {
			ice.Results = []ua.ResultHash{
				ua.ResultHash{DocumentURI: v.LastResponse.Results[0].URI, DocumentURIHash: urihash},
			}
		} else {
			return errors.New("ERR >>> Cannot convert sysurihash to string in interfaceChange event")
		}
	}

	err = v.UAClient.SendSearchEvent(ice)
	if err != nil {
		return err
	}
	return nil
}

// FindDocumentRankByTitle Looks through the last response to a query to find a document
// rank by his title
func (v *Visit) FindDocumentRankByTitle(toFind string) int {
	for i := 0; i < len(v.LastResponse.Results); i++ {
		if strings.Contains(strings.ToLower(v.LastResponse.Results[i].Title), strings.ToLower(toFind)) {
			return i
		}
	}
	return -1
}

// WaitBetweenActions Wait a random number of seconds between user actions
func WaitBetweenActions() {
	time.Sleep(time.Duration(rand.Intn(TIMEBETWEENACTIONS)+3) * time.Second)
}

// Min Function to return the minimal value between two integers, because Go "forgot"
// to code it...
func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetupNTO Function to instanciate with specific values for NTO demo queries
func (v *Visit) SetupNTO() {
	gbs := []*search.GroupByRequest{}
	q := &search.Query{
		Q:               "",
		CQ:              "",
		AQ:              "NOT @objecttype==(User,Case,CollaborationGroup) AND NOT @sysfiletype==(Folder, YouTubePlaylist, YouTubePlaylistItem)",
		NumberOfResults: 20,
		FirstResult:     0,
		Tab:             "All",
		GroupByRequests: gbs,
	}

	v.LastQuery = q

	v.OriginLevel1 = "communityCoveo"
	v.OriginLevel2 = "ALL"
}

// SetupGeneral Function to instanciate with non-specific values
func (v *Visit) SetupGeneral() {
	gbs := []*search.GroupByRequest{}
	q := &search.Query{
		Q:               "",
		CQ:              "",
		AQ:              "",
		NumberOfResults: 20,
		FirstResult:     0,
		Tab:             "All",
		GroupByRequests: gbs,
	}

	v.LastQuery = q

	v.OriginLevel1 = "ALL"
	v.OriginLevel2 = "ALL"
}
