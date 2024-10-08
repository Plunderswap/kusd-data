package main

import (
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	rpcEndpoint = "https://mainnet-v934-fireblocks.mainnet-20240103-ase1.zq1.network"

	contractAddress1 = "0xE9df5b4b1134A3aadf693Db999786699B016239e"
	mintAction       = "0x40C10F19"

	contractAddress2                = "0x7bAefF8996101048Ba905dB8695C8f77ae4e7631"
	depositToken1DistributionAction = "0x0800BA03"

	tokenOfInterest = "0xE9df5b4b1134A3aadf693Db999786699B016239e"
)

var (
	transferEventSig = []byte("Transfer(address,address,uint256)")
	transferTopic    = common.BytesToHash(transferEventSig)

	mintABI, _ = abi.JSON(strings.NewReader(`[{"inputs":[{"internalType":"address","name":"_receiver","type":"address"},{"internalType":"uint256","name":"_amount","type":"uint256"}],"name":"mint","outputs":[],"stateMutability":"nonpayable","type":"function"}]`))

	depositToken1DistributionABI, _ = abi.JSON(strings.NewReader(`[
		{
			"inputs": [
				{"internalType": "uint256", "name": "amount", "type": "uint256"}
			],
			"name": "depositToken1Distribution",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`))
)

func main() {
	client, err := rpc.Dial(rpcEndpoint)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()

	// Get the latest block number
	var latestBlockNumber string
	err = client.Call(&latestBlockNumber, "eth_blockNumber")
	if err != nil {
		log.Fatalf("Error fetching latest block number: %v", err)
	}
	endBlock, _ := hexutil.DecodeUint64(latestBlockNumber)

	// Calculate the start block (approximately 30 days ago)
	// Assuming an average block time of 15 seconds
	blocksPerDay := uint64(24 * 60 * 60 / 30)
	startBlock := endBlock - (30 * blocksPerDay)

	fmt.Printf("Searching from block %d to %d\n", startBlock, endBlock)

	searchBlocks(client, startBlock, endBlock)
}

func searchBlocks(client *rpc.Client, startBlock, endBlock uint64) {
	for blockNumber := endBlock; blockNumber >= startBlock; blockNumber-- {
		if blockNumber%1000 == 0 {
			fmt.Printf("Processing block %d\n", blockNumber)
		}

		var block map[string]interface{}
		err := client.Call(&block, "eth_getBlockByNumber", hexutil.EncodeUint64(blockNumber), true)
		if err != nil {
			log.Printf("Error fetching block %d: %v", blockNumber, err)
			continue
		}

		transactions := block["transactions"].([]interface{})
		for _, tx := range transactions {
			transaction, ok := tx.(map[string]interface{})
			if !ok {
				continue
			}

			to, ok := transaction["to"].(string)
			if !ok {
				continue
			}

			if strings.EqualFold(to, contractAddress1) || strings.EqualFold(to, contractAddress2) {
				input := transaction["input"].(string)
				if strings.HasPrefix(strings.ToLower(input), strings.ToLower(mintAction)) ||
					strings.HasPrefix(strings.ToLower(input), strings.ToLower(depositToken1DistributionAction)) {
					printDetailedTransactionInfo(client, transaction, blockNumber)
				}
			}
		}

		// Add a small delay to avoid overwhelming the node
		time.Sleep(50 * time.Millisecond)
	}
}

func printDetailedTransactionInfo(client *rpc.Client, tx map[string]interface{}, blockNumber uint64) {
	fmt.Printf("\nTransaction in block %d:\n", blockNumber)
	fmt.Printf("Hash: %s\n", tx["hash"])
	fmt.Printf("From: %s\n", tx["from"])
	fmt.Printf("To: %s\n", tx["to"])
	// fmt.Printf("Value: %s\n", tx["value"])

	input := tx["input"].(string)
	// fmt.Printf("Input: %s\n", input)

	if strings.HasPrefix(strings.ToLower(input), strings.ToLower(mintAction)) {
		fmt.Println("This is a mint transaction")
		decodeMintInput(input)
	} else if strings.HasPrefix(strings.ToLower(input), strings.ToLower(depositToken1DistributionAction)) {
		fmt.Println("This is a depositToken1Distribution transaction")
		decodeDepositToken1DistributionInput(input)
	}

	var receipt map[string]interface{}
	err := client.Call(&receipt, "eth_getTransactionReceipt", tx["hash"])
	if err != nil {
		log.Printf("Error getting transaction receipt: %v", err)
		return
	}

	analyzeTransferEvents(receipt)

	fmt.Println(strings.Repeat("-", 50))
}

func decodeMintInput(input string) {
	data, err := hexutil.Decode(input)
	if err != nil {
		log.Printf("Error decoding input: %v", err)
		return
	}

	method, err := mintABI.MethodById(data[:4])
	if err != nil {
		log.Printf("Error finding method: %v", err)
		return
	}

	args, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		log.Printf("Error unpacking arguments: %v", err)
		return
	}

	receiver := args[0].(common.Address)
	amount := new(big.Float).SetInt(args[1].(*big.Int))
	amount.Quo(amount, big.NewFloat(1e6)) // Divide by 10^6

	fmt.Printf("Mint - Receiver: %s, Amount: %.6f\n", receiver.Hex(), amount)
}

func decodeDepositToken1DistributionInput(input string) {
	data, err := hexutil.Decode(input)
	if err != nil {
		log.Printf("Error decoding input: %v", err)
		return
	}

	method, err := depositToken1DistributionABI.MethodById(data[:4])
	if err != nil {
		log.Printf("Error finding method with id %s: %v", hexutil.Encode(data[:4]), err)
		return
	}

	args, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		log.Printf("Error unpacking arguments: %v", err)
		return
	}

	amount := new(big.Float).SetInt(args[0].(*big.Int))
	amount.Quo(amount, big.NewFloat(1e6)) // Divide by 10^6

	fmt.Printf("DepositToken1Distribution - Amount: %.6f\n", amount)
}

func analyzeTransferEvents(receipt map[string]interface{}) {
	logs, ok := receipt["logs"].([]interface{})
	if !ok {
		log.Println("No logs found in receipt")
		return
	}

	for _, logEntry := range logs {
		log, ok := logEntry.(map[string]interface{})
		if !ok {
			continue
		}

		topics, ok := log["topics"].([]interface{})
		if !ok || len(topics) < 3 {
			continue
		}

		if topics[0].(string) == transferTopic.Hex() && strings.EqualFold(log["address"].(string), tokenOfInterest) {
			from := common.HexToAddress(topics[1].(string)).Hex()
			to := common.HexToAddress(topics[2].(string)).Hex()
			amount := new(big.Int)
			amount.SetString(log["data"].(string)[2:], 16)

			amountFloat := new(big.Float).SetInt(amount)
			amountFloat.Quo(amountFloat, big.NewFloat(1e6)) // Divide by 10^6

			fmt.Printf("Token Transfer - From: %s, To: %s, Amount: %.6f\n", from, to, amountFloat)
		}
	}
}
