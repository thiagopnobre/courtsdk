package courtsdk

import (
	"sync"
	"time"

	"github.com/gocolly/colly"
	"github.com/olivere/elastic"
)

//Engine is a structure used for define information of a engine.
type Engine struct {
	Court           string
	Base            string
	Failures        int
	Start           int
	End             int
	PageSize        int
	CurrentIndex    int
	Recoveries      int
	Done            bool
	EntryPoint      func(engine *Engine)
	ResponseChannel chan int
	Collector       *colly.Collector
	ElasticClient   *elastic.Client
	Lock            *sync.WaitGroup
}

//Jurisprudence is a structure used for serializing/deserializing data in Elasticsearch.
type Jurisprudence struct {
	Court            string    `json:"court"`
	DocumentType     string    `json:"document_type"`
	DocumentID       string    `json:"document_id"`
	IsEnabled        bool      `json:"is_enabled"`
	Checksum         string    `json:"checksum"`
	FullDocumentLink string    `json:"full_document_link"`
	Content          string    `json:"content"`
	JudgedAt         time.Time `json:"judged_at"`
	CreatedAt        time.Time `json:"inserted_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
