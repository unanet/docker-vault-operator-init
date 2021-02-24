package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	AwsSecretName          string `split_words:"true" required:"true"`
	VaultAddr              string `split_words:"true" required:"true"`
	VaultRecoveryShare     int    `split_words:"true" default:"5"`
	VaultRecoveryThreshold int    `split_words:"true" default:"3"`
}

func GetConfig() Config {
	c := Config{}
	err := envconfig.Process("", &c)
	if err != nil {
		panic(err)
	}

	return c
}

func main() {
	c := GetConfig()
	client := &http.Client{}
	var jsonStr = []byte(fmt.Sprintf(`{"recovery_shares":%d, "recovery_threshold": %d}`, c.VaultRecoveryShare, c.VaultRecoveryThreshold))
	retries := 0
	resp := &http.Response{}
	for retries < 10 {
		req, err := http.NewRequest("PUT", fmt.Sprintf("%s/v1/sys/init", c.VaultAddr), bytes.NewBuffer(jsonStr))
		if err != nil {
			log.Panic(err)
		}
		time.Sleep(10 * time.Second)

		resp, err = client.Do(req)
		if err != nil {
			log.Println(err)
			retries += 1
			continue
		}

		if resp != nil && resp.StatusCode != 200 {
			log.Printf("status: %d, retrying...", resp.StatusCode)
			retries += 1
			continue
		}
		break
	}

	if resp != nil && resp.StatusCode != 200 {
		log.Panicf("invalid status: %d", resp.StatusCode)
	}

	var respMap map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&respMap)
	if err != nil {
		log.Panic(err)
	}

	if _, ok := respMap["root_token"]; !ok {
		log.Panic("missing root token")
	}

	secretString, err := json.Marshal(respMap)
	if err != nil {
		log.Panic(err)
	}

	s := session.Must(session.NewSession())
	sm := secretsmanager.New(s)
	_, err = sm.CreateSecret(&secretsmanager.CreateSecretInput{
		Description:  aws.String("Initial Vault Root Token and Recovery Keys"),
		Name:         aws.String(c.AwsSecretName),
		SecretString: aws.String(string(secretString)),
	})

	if err != nil {
		log.Panic(err)
	}
}
