package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	VaultRecoveryShares    int    `split_words:"true" default:"5"`
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
	// we need to make sure that aws api is setup correctly before doing an init on vault,
	// so we create/update the secret to make sure we have proper access..
	s := session.Must(session.NewSession())
	sm := secretsmanager.New(s)
	vs, err := sm.DescribeSecret(&secretsmanager.DescribeSecretInput{
		SecretId: aws.String(c.AwsSecretName),
	})

	if err != nil {
		if _, ok := err.(*secretsmanager.ResourceNotFoundException); ok {
			_, err := sm.CreateSecret(&secretsmanager.CreateSecretInput{
				Description:  aws.String("Initial Vault Root Token and Recovery Keys"),
				Name:         aws.String(c.AwsSecretName),
				SecretString: aws.String("created"),
			})
			if err != nil {
				log.Panic(err)
			}
		} else {
			log.Panic(err)
		}
	} else {
		if vs.DeletedDate != nil {
			_, err = sm.RestoreSecret(&secretsmanager.RestoreSecretInput{
				SecretId: aws.String(c.AwsSecretName),
			})
			if err != nil {
				log.Panic(err)
			}
		}
	}

	_, err = sm.UpdateSecret(&secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(c.AwsSecretName),
		SecretString: aws.String("updated"),
	})

	if err != nil {
		log.Panic(err)
	}

	client := &http.Client{}
	var jsonStr = []byte(fmt.Sprintf(`{"recovery_shares":%d, "recovery_threshold": %d}`, c.VaultRecoveryShares, c.VaultRecoveryThreshold))
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
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			log.Printf("status: %d, body: %s, retrying...", resp.StatusCode, string(bodyBytes))
			retries += 1
			continue
		}
		break
	}

	if resp != nil && resp.StatusCode != 200 {
		log.Panicf("invalid status: %d", resp.StatusCode)
	}

	var respMap map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&respMap)
	if err != nil {
		log.Panic(err)
	}

	if _, ok := respMap["root_token"]; !ok {
		log.Panic("missing root token")
	}

	jsonStr = []byte(`{"type": "kv", "options": {"version": "2"}}`)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/sys/mounts/kv", c.VaultAddr), bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Panic(err)
	}
	req.Header.Add("X-Vault-Token", respMap["root_token"].(string))

	_, err = client.Do(req)
	if err != nil {
		log.Panic(err)
	}

	secretString, err := json.Marshal(respMap)
	if err != nil {
		log.Panic(err)
	}

	_, err = sm.UpdateSecret(&secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(c.AwsSecretName),
		SecretString: aws.String(string(secretString)),
	})
	if err != nil {
		log.Panic(err)
	}
}
