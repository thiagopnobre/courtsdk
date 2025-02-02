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
	engine.ResponseChannel = make(chan int)
	engine.PageSize = 1
	engine.MaxFailures = 25
	engine.MaxRecoveries = 5
	engine.UseDefaultChannelControl = true
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
	return func(engine *Engine) {
		ControlConfig["LastGoRoutineRange"] = start
		engine.Start = start
	}
}

// End set the end index
func End(end int) func(*Engine) {
	return func(engine *Engine) {
		engine.End = end
	}
}

// PageSize set the pagination size.
func PageSize(pageSize int) func(*Engine) {
	return func(engine *Engine) {
		engine.PageSize = pageSize
	}
}

// MaxFailures set the limit of failures of the current engine
func MaxFailures(maxFailures int) func(*Engine) {
	return func(engine *Engine) {
		engine.MaxFailures = maxFailures
	}
}

// MaxRecoveries set the limit of recoveries of the current engine
func MaxRecoveries(maxRecoveries int) func(*Engine) {
	return func(engine *Engine) {
		engine.MaxRecoveries = maxRecoveries
	}
}

// UseDefaultChannelControl ...
func UseDefaultChannelControl(useDefault bool) func(*Engine) {
	return func(engine *Engine) {
		engine.UseDefaultChannelControl = useDefault
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

// Setup is executed once before the engine entrypoint.
func Setup(setup func()) func(*Engine) {
	return func(engine *Engine) {
		engine.Setup = setup
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

// Concurrency set how many replicas and range (both greater than zero).
func Concurrency(maxReplicas int, replicaRange int) func(*Engine) {
	return func(engine *Engine) {
		if maxReplicas > 0 && replicaRange > 0 {
			engine.IsConcurrent = true
			engine.MaxReplicas = maxReplicas
			engine.ReplicaRange = replicaRange
		}
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

//GetDocumentType - returns the document type.
func (engine *Engine) GetDocumentType() string {
	switch engine.Base {
	case "baseAcordaos":
		return "Acórdãos"
	case "baseSumulas":
		return "Súmulas"
	case "baseSumulasVinculantes":
		return "Súmulas Vinculantes"
	case "basePresidencia":
		return "Decisões da Presidência"
	case "baseRepercussao":
		return "Repercussão Geral"
	case "basePrecedentes":
		return "Precedentes Normativos"
	default:
		return "CUSTOM[" + engine.Base + "]"
	}
}

func (engine *Engine) channelControl() {
	if engine.UseDefaultChannelControl {
		go func() {
			defer engine.Lock.Done()
			for {
				select {
				case status := <-engine.ResponseChannel:
					engine.handleChannelStatus(status)
				default:
					if engine.shouldStop() {
						return
					}
				}
			}
		}()
		engine.Lock.Add(1)
	}
}

func (engine *Engine) shouldStop() bool {
	if engine.Failures >= engine.MaxFailures {
		log.Println("[FAILED] The engine ["+engine.Base+"] has failed ", engine.Failures, " times.")
		return true
	} else if engine.IsConcurrent && engine.CurrentIndex > engine.End {
		return true
	} else if engine.done {
		return true
	}
	return false
}

func (engine *Engine) handleChannelStatus(status int) {
	if status != http.StatusOK {
		engine.Failures++
	} else if engine.Failures > 0 {
		engine.Failures--
	}
}

func (engine *Engine) runAsSequential() {
	engine.InitElastic()
	if engine.ConnectedToIndex() {
		engine.Recoveries = 0
		for engine.Recoveries <= engine.MaxRecoveries {
			engine.Collector = GetDefaultcollector()
			engine.channelControl()
			engine.EntryPoint(engine)
			engine.Lock.Wait()
			if engine.done {
				engine.logSuccess()
				return
			}
			engine.logFailure()
			engine.setRecoveryStart()
			time.Sleep(ControlConfig["ActionDelay"].(time.Duration) * time.Second)
			engine.Failures = 0
			engine.Recoveries++
		}
	}
}

func (engine *Engine) runAsConcurrent() {
	activeEngines := 0
	maxEngines := engine.MaxReplicas
	activeEnginesChannel := make(chan int)
	maxEnginesChannel := make(chan int)
	mutex := sync.Mutex{}
	for {
		if activeEngines == 0 && maxEngines == 0 {
			return
		}
		select {
		case value := <-activeEnginesChannel:
			activeEngines += value
		case value := <-maxEnginesChannel:
			maxEngines += value
		default:
			if activeEngines < maxEngines {
				activeEngines++
				go engine.spawnEngine(activeEnginesChannel, maxEnginesChannel, &mutex)
			}
		}
	}
}

func (engine Engine) spawnEngine(activeEnginesChannel chan int, maxEnginesChannel chan int, mutex *sync.Mutex) {
	engine.InitElastic()
	mutex.Lock()
	connectedToIndex := engine.ConnectedToIndex()
	mutex.Unlock()
	if connectedToIndex {
		engine.setRange(mutex)
		for engine.Recoveries <= engine.MaxRecoveries {
			engine.Collector = GetDefaultcollector()
			engine.channelControl()
			engine.EntryPoint(&engine)
			engine.Lock.Wait()
			if engine.done {
				engine.logSuccess()
				activeEnginesChannel <- -1
				return
			}
			engine.logFailure()
			engine.setRecoveryStart()
			time.Sleep(ControlConfig["ActionDelay"].(time.Duration) * time.Second)
			engine.Failures = 0
			engine.Recoveries++
		}
		maxEnginesChannel <- -1
	}
	activeEnginesChannel <- -1
}

func (engine *Engine) setRange(mutex *sync.Mutex) {
	lastRange := ControlConfig["LastGoRoutineRange"].(int)
	if lastRange > -1 {
		engine.Start = lastRange
	}
	engine.End = engine.Start + engine.ReplicaRange - 1
	mutex.Lock()
	ControlConfig["LastGoRoutineRange"] = engine.End + 1
	mutex.Unlock()
	log.Println("[INFO] New Engine replica RANGE: ", engine.Start, " to", engine.End)
}

func (engine *Engine) setRecoveryStart() {
	if engine.CurrentIndex != 0 {
		engine.Start = engine.CurrentIndex - (engine.Failures * engine.PageSize)
	}
}

func (engine Engine) logSuccess() {
	str := "[ENGINE] COURT -> " + engine.Court
	str += " BASE -> " + engine.Base + " ended successfully."
	log.Println(str)
}

func (engine Engine) logFailure() {
	str := "[ENGINE] COURT -> " + engine.Court
	str += " BASE -> " + engine.Base + " " + strconv.Itoa(engine.MaxFailures) + " times."
	str += " Last ID requested: " + strconv.Itoa(engine.CurrentIndex)
	str += " Trying to recover from index -> " + strconv.Itoa(engine.Start)
	log.Println(str)
}

func (engine Engine) doSetup() {
	if engine.Setup == nil {
		DebugPrint("[ENGINE] COURT -> " + engine.Court + " BASE -> " + engine.Base + " has no Setup function.")
		return
	}
	engine.Setup()
}

//Done - Set the done property to true.
func (engine *Engine) Done() {
	engine.done = true
}
