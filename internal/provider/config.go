package provider

import (
	"database/sql"
	"fmt"
	"sync"
)

type Config struct {
	dsnTemplate     string
	dbRegistry      map[string]*sql.DB
	dbRegistryMutex sync.Mutex
}

func newConfig(dsnTemplate string) *Config {
	return &Config{
		dbRegistry:  make(map[string]*sql.DB),
		dsnTemplate: dsnTemplate,
	}
}

func (c *Config) connectToMySQLDb(dbName string) (*sql.DB, error) {
	dsn := fmt.Sprintf(c.dsnTemplate, dbName)
	return c.connectToMySQL(dsn)
}

func (c *Config) connectToMySQL(dsn string) (*sql.DB, error) {
	c.dbRegistryMutex.Lock()
	defer c.dbRegistryMutex.Unlock()

	if c.dbRegistry[dsn] != nil {
		return c.dbRegistry[dsn], nil
	}

	db, err := sql.Open("cloudsql-mysql", dsn)
	if err != nil {
		return nil, err
	}

	c.dbRegistry[dsn] = db
	return c.dbRegistry[dsn], nil
}
