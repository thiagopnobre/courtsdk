package courtsdk

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gocolly/colly"
	"github.com/olivere/elastic"
)

// NewEngine creates a new Engine instance with default configuration
func NewEngine(options ...func(*Engine)) *Engine {
	engine := &Engine{}
	engine.Collector = GetDefaultcollector()
	engine.ResponseChannel = make(chan int)
	engine.Done = false
	engine.PageSize = 1
	var wg sync.WaitGroup
	engine.Lock = &wg
	for _, attr := range options {
		attr(engine)
	}
	return engine
}

// Court set the current court.
func Court(court string) func(*Engine) {
	return func(engine *Engine) {
		engine.Court = court
	}
}

// Base set the current base.
func Base(base string) func(*Engine) {
	return func(engine *Engine) {
		engine.Base = base
	}
}

// Start set the start index
func Start(start int) func(*Engine) {
	return func(eng *Engine) {
		if ControlConfig["isConcurrent"].(bool) {
			ControlConfig["LastGoRoutineRange"] = start - 1
		}
		eng.Start = start
	}
}

// End set the end index
func End(end int) func(*Engine) {
	return func(eng *Engine) {
		eng.End = end
	}
}

// PageSize set the pagination size.
func PageSize(pageSize int) func(*Engine) {
	return func(engine *Engine) {
		engine.PageSize = pageSize
	}
}

// Collector set the engine private collector (colly)
func Collector(collector *colly.Collector) func(*Engine) {
	return func(engine *Engine) {
		engine.Collector = collector
	}
}

// ElasticClient set the engine private elasticsearch client
func ElasticClient(client *elastic.Client) func(*Engine) {
	return func(engine *Engine) {
		engine.ElasticClient = client
	}
}

// EntryPoint set a function to start the engine.
func EntryPoint(entry func(engine *Engine)) func(*Engine) {
	return func(engine *Engine) {
		engine.EntryPoint = entry
	}
}

// ResponseChannel set an own private channel for the engine.
func ResponseChannel(responseChannel chan int) func(*Engine) {
	return func(engine *Engine) {
		engine.ResponseChannel = responseChannel
	}
}

// Lock set a private WaitGroup for the engine.
func Lock(lock *sync.WaitGroup) func(*Engine) {
	return func(engine *Engine) {
		engine.Lock = lock
	}
}

//InitElastic - Initialize an Elasticsearch client with Elastic configs.
func (engine *Engine) InitElastic() {
	var err error
	elasticFullURL := ElasticConfig["URL"].(string) + ":" + strconv.Itoa(ElasticConfig["Port"].(int))
	engine.ElasticClient, err = elastic.NewClient(elastic.SetSniff(false), elastic.SetURL(elasticFullURL))
	if err != nil {
		log.Println("[FAILED] Connect to Elasticsearch.", err)
		log.Println("[WARNING] Retrying in ", strconv.Itoa(ElasticConfig["RetryConnectionDelay"].(int)), " seconds...")
		time.Sleep(time.Duration(ElasticConfig["RetryConnectionDelay"].(int)) * time.Second)
		engine.InitElastic()
		return
	}
	engine.pingElasticSearch(elasticFullURL)
}

func (engine *Engine) pingElasticSearch(elasticFullURL string) {
	context, cancelContext := GetNewContext()
	defer cancelContext()
	info, code, err := engine.ElasticClient.Ping(elasticFullURL).Do(context)
	if err != nil {
		log.Println("[FAILED] Ping to Elasticsearch.", err)
		log.Println("[WARNING] Retrying in ", strconv.Itoa(ElasticConfig["RetryPingDelay"].(int)), " seconds...")
		time.Sleep(time.Duration(ElasticConfig["RetryPingDelay"].(int)) * time.Second)
		engine.InitElastic()
		return
	}
	log.Printf("[SUCCESS] Elasticsearch returned with code %d and version %s\n", code, info.Version.Number)
}

//ConnectedToIndex - Check if the given index exist.
func (engine *Engine) ConnectedToIndex() bool {
	index := ElasticConfig["Index"].(string)
	context, cancelContext := GetNewContext()
	defer cancelContext()
	exists, err := engine.ElasticClient.IndexExists(index).Do(context)
	if err != nil {
		log.Println("[FAILED] Unable to connect to index -> ["+index+"]", err)
		return false
	}
	if !exists {
		log.Println("[WARNING] Index -> [" + index + "] not found. Attempting to create...")
		createIndex, err := engine.ElasticClient.CreateIndex(index).BodyString(GetElasticMapping()).Do(context)
		if err != nil {
			log.Println("[FAILED] Create index -> ["+index+"].", err)
			return false
		}
		if !createIndex.Acknowledged {
			log.Println("[WARNING] Index -> [" + index + "] was created, but not acknowledged.")
			return false
		}
		log.Println("[SUCCESS] Index -> [" + index + "] was created and acknowledged.")
		return true
	}
	log.Println("[SUCCESS] Index -> [" + index + "] was found, sending data to it...")
	return true
}

//Persist - send data to Elasticsearch.
func (engine *Engine) Persist(jurisprudence Jurisprudence) {
	uid := jurisprudence.Court + "-" + engine.Base + "-" + jurisprudence.DocumentID
	context, cancelContext := GetNewContext()
	defer cancelContext()
	_, err := engine.ElasticClient.Index().
		Index(ElasticConfig["Index"].(string)).
		Type("_doc").
		Id(uid).
		BodyJson(jurisprudence).
		Do(context)
	if err != nil {
		log.Println("[FAILED][CREATE] Save document ["+jurisprudence.DocumentID+"]["+jurisprudence.DocumentType+"]:", err)
		engine.ResponseChannel <- http.StatusInternalServerError
	}
	engine.ResponseChannel <- http.StatusOK
}
