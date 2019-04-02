package main

import (
	"fmt"
	"github.com/eriklupander/tradfri-go/router"
	"github.com/eriklupander/tradfri-go/tradfri"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"

	"sync"
	"time"
)

var configFlags = pflag.NewFlagSet("config", pflag.ExitOnError)
var commandFlags = pflag.NewFlagSet("commands", pflag.ExitOnError)

func init() {
	configFlags.String("gateway_ip", "", "ip to your gateway. No protocol or port here!")
	configFlags.String("gateway_address", "", "address to your gateway. Including port here!")
	configFlags.String("psk", "", "Pre-shared key on bottom of Gateway")
	configFlags.String("client_id", "", "Your client id, make something up or use the NNN-NNN-NNN on the bottom of your Gateway")

	commandFlags.Bool("server", false, "Start in server mode?")
	commandFlags.Bool("authenticate", false, "Perform PSK exchange")
	commandFlags.String("get", "", "URL to GET")
	commandFlags.String("put", "", "URL to PUT")
	commandFlags.String("payload", "", "payload for PUT")
	
	commandFlags.AddFlagSet(configFlags)
	commandFlags.Parse(os.Args[1:])

	viper.BindPFlags(configFlags)
	viper.AutomaticEnv()
	viper.AddConfigPath(".") // e.g. reads ./config.json or config.yaml
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Println(err)
		fmt.Println("You probably have to run --authenticate first")
	}
	viper.RegisterAlias("pre_shared_key", "psk")
}

func main() {
	gatewayAddress := viper.GetString("gateway_address")
	if gatewayAddress == "" {
		gatewayAddress = viper.GetString("gateway_ip") + ":5684"
	}
	psk := viper.GetString("psk")
	clientID := viper.GetString("client_id")
	serverMode, _ := commandFlags.GetBool("server")
	authenticate, _ := commandFlags.GetBool("authenticate")
	get, getErr := commandFlags.GetString("get")
	put, putErr := commandFlags.GetString("put")
	payload, _ := commandFlags.GetString("payload")

	// Handle the special authenticate use-case
	if authenticate {
		performTokenExchange(gatewayAddress, clientID, psk)
		return
	}

	checkRequiredConfig()

	// Check running mode...
	if serverMode {
		fmt.Println("Running in server mode on :8080")
		go router.SetupChi(tradfri.NewTradfriClient(gatewayAddress, clientID, psk))

		wg := sync.WaitGroup{}
		wg.Add(1)
		wg.Wait()
	} else {
		// client mode
		if getErr == nil && get != "" {
			resp, _ := tradfri.NewTradfriClient(gatewayAddress, clientID, psk).Get(get)
			fmt.Printf("%v", string(resp.Payload))
		} else if putErr == nil && put != "" {
			resp, _ := tradfri.NewTradfriClient(gatewayAddress, clientID, psk).Put(put, payload)
			fmt.Printf("%v", string(resp.Payload))
		} else {
			fmt.Println("No client operation was specified, supported one(s) are: get, put, authenticate")
		}
	}

}

func checkRequiredConfig() {
	if viper.GetString("PRE_SHARED_KEY") == "" {
		fail("Unable to resolve PRE_SHARED_KEY from env-var or psk.key file")
	}
	if viper.GetString("CLIENT_ID") == "" {
		fail("Unable to resolve CLIENT_ID from env-var or psk.key file")
	}
}

func performTokenExchange(gatewayAddress, clientID, psk string) {
	if len(clientID) < 1 || len(psk) < 10 {
		fail("Both clientID and psk args must be specified when performing key exchange")
	}

	done := make(chan bool)
	defer func() { done <- true }()
	go func() {
		select {
		case <-time.After(time.Second * 5):
			fmt.Println("(Please note that the key exchange may appear to be stuck at \"Connecting to peer at\" if the PSK from the bottom of your Gateway is not entered correctly.)")
		case <-done:
		}
	}()

	// Note that we hard-code "Client_identity" here before creating the DTLS client,
	// required when performing token exchange
	dtlsClient := tradfri.NewTradfriClient(gatewayAddress, "Client_identity", psk)

	authToken, err := dtlsClient.AuthExchange(clientID)
	if err != nil {
		fail(err.Error())
	}
	viper.Set("client_id", clientID)
	viper.Set("gateway_address", gatewayAddress)
	viper.Set("psk", authToken.Token)
	err = viper.WriteConfigAs("config.json")
	if err != nil {
		fail(err.Error())
	}
	fmt.Println("Your new PSK and clientID has been written to psk.key, keep this file safe!")
}

func fail(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}