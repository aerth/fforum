package forum

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/boltdb/bolt"
)

var db *bolt.DB
var ran *rand.Rand

func init() {
	ran = rand.New(rand.NewSource(time.Now().UnixNano())) // new random source
}

func Init(location string) {
	db, err = bolt.Open(location, 0666, nil)
	if err != nil {
		log.Fatal(err)
	}

	// go func() {
	// 	log.Println("Starting stats loop")
	// 	for {
	// 		fmt.Println("stats:", db.Stats().TxStats.WriteTime)
	// 		time.Sleep(10 * time.Second)
	// 		// create buckets if they dont exist
	// 	}
	// }()
	buckets := []string{"forumTopic", "forumCategory", "forumReply"}
	for _, bucket := range buckets {
		err = Write(bucket, "", nil)
		if err != nil {
			log.Println(err)
		}
	}

}

// Close safely close BoltDB
func Close() {
	if err := db.Close(); err != nil {
		log.Fatal(err)
	}
}

func Delete(bucket, key string) error {
	// Delete the key in a different write transaction.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucket)).Delete([]byte(key))
	}); err != nil {
		return err
	}
	return nil
}

// Write: Insert data into a bucket.
func Write(bucket, key string, value []byte) error {
	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}

		if key != "" { // blank key just creates the bucket. nil value still gets stored if key is named.
			if err := b.Put([]byte(key), value); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}
	return nil
}

// Read: retrieve []byte(value) of bucket[key]
func Read(bucket, key string) []byte {
	if bucket == "" || key == "" {
		return nil
	}

	var v []byte
	err = db.View(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte(bucket)) == nil {
			return nil
		}
		v = tx.Bucket([]byte(bucket)).Get([]byte(key))
		return nil // no error
	})
	if err != nil {
		log.Println(err)
		return nil

	}
	return v
}

// Topic knows nothing about categories. they all exist in the same bucket.
type Topic struct {
	ID       string // unique 64
	Owner    string // owner id
	Title    string // owner id
	Body     string
	Category string // id 32
}
type Reply struct {
	ID    string // unique 64
	To    string // Topic.ID
	Owner string // replier
	Body  string
}

// Categories have a creator, Name, and a list of post IDs.
type Category struct {
	ID, Creator, Name string // ID is unique 32
	Topics            []string
}

// Save lol
func (p Topic) Save() error {

	// get category
	if p.Category == "" {
		p.Category = "invalid"
	}
	b := Read("forumCategory", p.Category)
	var cat Category
	err = json.Unmarshal(b, &cat)
	if err != nil {
		return err
	}
	// add to category
	cat.Topics = append(cat.Topics, p.ID)
	b, err = json.Marshal(cat)
	if err != nil {
		return err
	}
	err = Write("forumCategory", p.Category, b)
	if err != nil {
		return err
	}

	// Turn to json
	b, err = json.Marshal(p)
	if err != nil {
		return err
	}

	// Write it
	err = Write("forumTopic", p.ID, b)
	if err != nil {
		return err
	}

	return nil
}

// Save lol
func (c Category) Save() error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	err = Write("forumCategory", c.ID, b)
	if err != nil {
		return err
	}
	return nil
}

// Save lol
func (r Reply) Save() error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	err = Write("forumReply", r.ID, b)
	if err != nil {
		return err
	}
	return nil
}

func (p Topic) Delete() error {
	err := Delete("forumTopic", p.ID)
	if err != nil {
		return err
	}
	return nil
}

func (r Reply) Delete() error {
	err := Delete("forumReply", r.ID)
	if err != nil {
		return err
	}
	return nil
}

func (c Category) Delete() error {
	err := Delete("forumCategory", c.Name)
	if err != nil {
		return err
	}
	return nil
}

// New topic
func NewTopic() *Topic {
	p := new(Topic)
	p.ID = uniqueTopic()
	return p
}
func (p *Topic) NewReply() *Reply {
	r := new(Reply)
	r.To = p.ID
	r.ID = uniqueReply()
	return r
}
func (p *Topic) Replies() map[string]Reply {
	return AllRepliesOf(p.ID)

}

// New category
func NewCategory() *Category {
	c := new(Category)
	c.ID = uniqueCat()
	return c
}

// Read a topic
func ReadTopic(id string) (*Topic, error) {
	post := new(Topic)
	b := Read("forumTopic", id)
	if b == nil {
		return post, errors.New("No topic")
	}
	err := json.Unmarshal(b, post)
	if err != nil {
		return post, nil
	}
	return post, nil
}

// random character
func random(n int) string {
	runes := []rune("abcdefg1234567890") // mostly numbers
	b := make([]rune, n)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return strings.TrimSpace(string(b))
}

// generate unique
func uniqueTopic() string {
	u := random(64)
	try := Read("forumTopic", u)
	if try == nil {
		return u
	}
	if string(try) != "" {
		return uniqueTopic()
	}

	return u

}

func uniqueCat() string {
	u := random(32)
	try := Read("forumCategory", u)
	if try == nil {
		return u
	}
	if string(try) != "" {
		return uniqueCat()
	}

	return u

}

func uniqueReply() string {
	u := random(64)
	try := Read("forumReply", u)
	if try == nil {
		return u
	}
	if string(try) != "" {
		return uniqueReply()
	}
	log.Println(u)
	return u

}

var err error

func ListCategories() map[string]Category {
	var cats = map[string]Category{}
	var i = 0

	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("forumCategory"))
		if err != nil {
			return err
		}

		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			i++
			var cat Category
			err = json.Unmarshal(v, &cat)
			if err != nil {
				return err
			}
			// Insert into user map[id string]user
			cats[string(k)] = cat
			return nil
		}); err != nil {
			log.Println(err)
			os.Exit(1)
			return err
		}
		return nil
	}); err != nil {
		log.Println(err)
	}
	return cats
}
func ListTopicsAll() map[string]Topic {
	var topics = map[string]Topic{}
	var i = 0

	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("forumTopic"))
		if err != nil {
			return err
		}

		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			i++
			var topic Topic
			err = json.Unmarshal(v, &topic)
			if err != nil {
				return err
			}
			// Insert into user map[id string]user
			topics[string(k)] = topic
			return nil
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	return topics
}
func ListTopicsOf(s string) map[string]Topic {
	var topics = map[string]Topic{}
	var i = 0

	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("forumTopic"))
		if err != nil {
			return err
		}

		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			i++
			var topic Topic
			err = json.Unmarshal(v, &topic)
			if err != nil {
				return err
			}
			// Insert into user map[id string]user
			if topic.Category == s {
				topics[string(k)] = topic
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	return topics
}

// AllRepliesOf lol
func AllRepliesOf(tid string) map[string]Reply {
	var replies = map[string]Reply{}
	var i = 0

	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("forumReply"))
		if err != nil {
			log.Println(err)
			return err

		}

		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			i++
			var reply Reply
			err = json.Unmarshal(v, &reply)
			if err != nil {
				log.Println(err)
				return err
			}
			// Insert into user map[id string]user
			if reply.To == tid {
				// log.Println("Got a match for reply:", tid, reply.ID)
				replies[string(k)] = reply
			} else {
				// log.Println("Got no match for reply:", tid, reply.ID)
			}
			return nil
		}); err != nil {
			log.Println(err)
			return err
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	return replies
}

func ReadReply(rid string) Reply {
	var reply Reply
	b := Read("forumReply", rid)
	if b == nil {
		return reply
	}
	err := json.Unmarshal(b, &reply)
	if err != nil {
		return reply
	}
	return reply
}

func AllReplies() map[string]Reply {
	var replies = map[string]Reply{}
	var i = 0

	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("forumReply"))
		if err != nil {
			log.Println(err)
			return err

		}

		// Iterate over items in sorted key order.
		if err := b.ForEach(func(k, v []byte) error {
			i++
			var reply Reply
			err = json.Unmarshal(v, &reply)
			if err != nil {
				log.Println(err)
				return err
			}
			// Insert into user map[id string]user

			replies[string(k)] = reply

			return nil
		}); err != nil {
			log.Println(err)
			return err
		}
		return nil
	}); err != nil {
		log.Println(err)
		return replies
	}
	return replies
}
