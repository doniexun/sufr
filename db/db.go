package db

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"

	"github.com/boltdb/bolt"
	"github.com/kyleterry/sufr/config"
)

// SufrDB is a BoltDB wrapper that provides a SUFR specific interface to the DB
type SufrDB struct {
	path     string
	database *bolt.DB
}

// SufrItem is an interface to describe how to serialize a storable item for the
// Sufr Database
type SufrItem interface {
	GetID() uint64
	SetID(uint64)
	Type() string
	Serialize() ([]byte, error)
}

// New creates and returns a new pointer to a SufrDB struct
func New(path string) *SufrDB {
	return &SufrDB{path: path}
}

//Open will open the bolt database and panic on error
func (sdb *SufrDB) Open() error {
	if sdb.database != nil {
		return config.ErrDatabaseAlreadyOpen
	}
	db, err := bolt.Open(sdb.path, 0600, nil)
	if err != nil {
		return err
	}

	sdb.database = db

	// Make sure the sufr bucket exists so we can use it later
	err = sdb.database.Update(func(tx *bolt.Tx) error {
		log.Println("Creating database buckets")
		b, err := tx.CreateBucketIfNotExists([]byte(config.BucketNameRoot))
		if err != nil {
			return err
		}

		_, err = b.CreateBucketIfNotExists([]byte(config.BucketNameURL))
		if err != nil {
			return err
		}

		_, err = b.CreateBucketIfNotExists([]byte(config.BucketNameTag))
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// Close will close the BoltDB instance
func (sdb *SufrDB) Close() {
	sdb.database.Close()
}

// Put will create or update a SufrItem
func (sdb *SufrDB) Put(item SufrItem) error {
	err := sdb.database.Update(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(config.BucketNameRoot))
		b := rootBucket.Bucket([]byte(item.Type()))
		id := item.GetID()
		if id == 0 {
			id, _ = b.NextSequence()
			item.SetID(id)
		}
		content, err := item.Serialize()
		if err != nil {
			return err
		}
		b.Put(itob(id), content)
		return nil
	})
	return err
}

// Get will return raw bytes for an item at `id` or return an error
func (sdb *SufrDB) Get(id uint64, bucket string) ([]byte, error) {
	var item []byte
	err := sdb.database.View(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(config.BucketNameRoot))
		b := rootBucket.Bucket([]byte(bucket))
		item = b.Get(itob(id))
		return nil
	})
	return item, err
}

// GetAll is used to fetch all of the recods for a particular bucket
// Returns a slice of []byte or an error
func (sdb *SufrDB) GetAll(bucket string) ([][]byte, error) {
	items := [][]byte{}
	err := sdb.database.View(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(config.BucketNameRoot))
		b := rootBucket.Bucket([]byte(bucket))
		b.ForEach(func(_, v []byte) error {
			items = append(items, v)
			return nil
		})

		return nil
	})
	return items, err
}

// Delete will return raw bytes for an item at `id` or return an error
func (sdb *SufrDB) Delete(id uint64, bucket string) error {
	err := sdb.database.Update(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(config.BucketNameRoot))
		b := rootBucket.Bucket([]byte(bucket))
		return b.Delete(itob(id))
	})
	return err
}

// GetValuesByField will find objects who's `fieldname` value matches any of the `values`
// If any of the objects are lacking `fieldname` when deserialized, then it returns an error
// Return [][]byte, a []string of objects not found or an error
func (sdb *SufrDB) GetValuesByField(fieldname, bucket string, values ...string) ([][]byte, []string, error) {
	items := [][]byte{}
	err := sdb.database.Update(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket([]byte(config.BucketNameRoot))
		b := rootBucket.Bucket([]byte(bucket))
		err := b.ForEach(func(k []byte, v []byte) error {
			j := make(map[string]interface{})
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}
			if _, ok := j[fieldname]; !ok {
				return fmt.Errorf("Field `%s` doesn't exist", fieldname)
			}
			valuestring := j[fieldname].(string)
			for i, need := range values {
				if need == valuestring {
					items = append(items, v)
					// If we find a match, remove this one from the values slice so we can return
					// it as notfound values
					values = append(values[:i], values[i+1:]...)
				}
			}
			return nil
		})
		return err
	})
	return items, values, err
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
