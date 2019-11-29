// Copyright 2019 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package store

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/mendersoftware/go-lib-micro/config"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	dconfig "github.com/mendersoftware/workflows/config"
	"github.com/mendersoftware/workflows/model"
)

// MongoClient is a package specific mongo client
type MongoClient struct {
	mongo.Client
}

// NewMongoClient returns a mongo client
func NewMongoClient(ctx context.Context, c config.Reader) (*MongoClient, error) {

	clientOptions := mopts.Client()
	mongoURL := c.GetString(dconfig.SettingMongo)
	if !strings.Contains(mongoURL, "://") {
		return nil, errors.Errorf("Invalid mongoURL %q: missing schema.",
			mongoURL)
	}
	clientOptions.ApplyURI(mongoURL)

	username := c.GetString(dconfig.SettingDbUsername)
	if username != "" {
		credentials := mopts.Credential{
			Username: c.GetString(dconfig.SettingDbUsername),
		}
		password := c.GetString(dconfig.SettingDbPassword)
		if password != "" {
			credentials.Password = password
			credentials.PasswordSet = true
		}
		clientOptions.SetAuth(credentials)
	}

	if c.GetBool(dconfig.SettingDbSSL) {
		tlsConfig := &tls.Config{}
		tlsConfig.InsecureSkipVerify = c.GetBool(dconfig.SettingDbSSLSkipVerify)
		clientOptions.SetTLSConfig(tlsConfig)
	}

	// Set writeconcern to acknowlage after write has propagated to the
	// mongod instance and commited to the file system journal.
	var wc *writeconcern.WriteConcern
	wc.WithOptions(writeconcern.W(1), writeconcern.J(true))
	clientOptions.SetWriteConcern(wc)

	if clientOptions.ReplicaSet != nil {
		clientOptions.SetReadConcern(readconcern.Linearizable())
	}

	// Set 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to mongo server")
	}

	// Validate connection
	if err = client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "Error reaching mongo server")
	}

	mongoClient := MongoClient{Client: *client}
	return &mongoClient, nil
}

// DataStoreInterface for DataStore  services
type DataStoreInterface interface {
	InsertJob(ctx context.Context, job *model.Job) (*model.Job, error)
	GetJobs(ctx context.Context) <-chan *model.Job
	Shutdown()
}

// DataStore is the data storage service
type DataStore struct {
	client   *MongoClient
	dbName   string
	shutdown bool
}

// NewDataStoreWithClient initializes a DataStore object
func NewDataStoreWithClient(client *MongoClient, c config.Reader) *DataStore {
	dbName := c.GetString(dconfig.SettingDbName)

	return &DataStore{
		client:   client,
		dbName:   dbName,
		shutdown: false,
	}
}

// InsertJob inserts the job in the queue
func (db *DataStore) InsertJob(ctx context.Context, job *model.Job) (*model.Job, error) {

	var inputParameters []model.InputParameter

	for _, param := range job.InputParameters {
		inputParameters = append(inputParameters, model.InputParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	collection := db.client.Database(db.dbName).Collection(JobsCollectionName)
	result, err := collection.InsertOne(ctx, bson.M{
		"workflow_name":    job.WorkflowName,
		"input_parameters": inputParameters,
	})
	if err != nil {
		return nil, err
	}

	job.ID = result.InsertedID.(primitive.ObjectID).Hex()

	return job, nil
}

// GetJobs returns a channel of Jobs
func (db *DataStore) GetJobs(ctx context.Context) <-chan *model.Job {
	var channel = make(chan *model.Job)

	go func() {
		findOptions := &options.FindOptions{}
		findOptions.SetCursorType(options.TailableAwait)
		findOptions.SetMaxTime(10 * time.Second)
		findOptions.SetBatchSize(100)

		query := bson.M{}

		collection := db.client.Database(db.dbName).Collection(JobsCollectionName)
		cur, err := collection.Find(ctx, query, findOptions)
		if err != nil {
			fmt.Println(err)
			channel <- nil
			return
		}

		defer cur.Close(ctx)

		for {
			for cur.TryNext(ctx) {
				job := new(model.Job)
				err = cur.Decode(job)
				if err == nil {
					channel <- job
				}
			}
			if db.shutdown {
				break
			}
		}

		channel <- nil
	}()

	return channel
}

// Shutdown shuts down the datastore GetJobs process
func (db *DataStore) Shutdown() {
	db.shutdown = true
}
