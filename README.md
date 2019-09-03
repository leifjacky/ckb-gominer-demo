## Nervos(CKB) cpuminer written in Golang

Test only.

### usage

```bash
go get -v github.com/leifjacky/ckb-gominer-demo
cd $GOPATH/src/github.com/leifjacky/ckb-gominer-demo
go run *.go
```

## Nervos(CKB) stratum protocol

### mining.subscribe

- params: ["agent", null]
- result: [null, "nonce prefix", "nonce2 size"]

```json
request:
{
	"id": 1,
	"method": "mining.subscribe",
	"params": ["ckbminer-v1.0.0", null]
}

response:
{
	"id": 1,
	"result": [null, "3e29d5", 5],
	"error": null
}
```

nonce1 is first part of the block header nonce (in hex).

By protocol, CKB's nonce is 8 bytes long. The miner will pick nonce2 such that len(nonce2) = 8 - len(nonce1) = 8 - 3 = 5 bytes.



### mining.authorize

- params: ["username", "password"]
- result: true

```json
{
	"id": 2,
	"method": "mining.authorize",
	"params": ["ckb1qyq2znu0gempdahctxsm49sa9jdzq9vnka7qt9ntff", "x"]
} 

{"id":2,"result":true,"error":null}
```



### mining.set_target

- params: ["32bytes target in hex"]

```json
{
	"id": null,
	"method": "mining.set_target",
	"params": ["0020010000000000000000000000000000000000000000000000000000000000"]
}
```



### mining.notify

- params: ["jobId", "header hash", height, "parent hash", cleanJob]

```json
{
	"id": null,
	"method": "mining.notify",
	"params": ["17282f3f", "2860e9966c50829a76e650dc4abdf49c925d2fd116eab69cd7bc1ae6673225ef", 21728, "ebf5cca491c4760c1b4d9306e6aed35b17d773ab60650ed58974a84b2d0fb82c", true]
}
```



### mining.submit

- params: [ "username", "jobId", "nonce2" ]
- result: true / false

```json
{
	"id": 102,
	"method": "mining.submit",
	"params": ["tmAfQj8jv7SMuCpzSkPMTR8v8AaKzQkJ7P2.worker1", "17282f3f", "eaf71970c0"]
}

{"id":102,"result":true,"error":null}    // accepted share response
{"id":102,"result":false,"error":[21,"low difficulty",null]}  // rejected share response
```

nonce2 is the second part of the nonce.

```json
In this example

nonce = nonce1 + nonce2 = 3e29d5eaf71970c0

headerHash = 2860e9966c50829a76e650dc4abdf49c925d2fd116eab69cd7bc1ae6673225ef   (32 bytes without nonce)

headerHashWithNonce = nonce + headerHash = 3e29d5eaf71970c02860e9966c50829a76e650dc4abdf49c925d2fd116eab69cd7bc1ae6673225ef   (40 bytes with nonce)

powHash = eaglesonghash(headerHashWithNonce) = 0000dfd9214a52ee0860d988e66c1799847744ef43155b8e00c3f6e3948dbb93
```


