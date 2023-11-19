
# Run Node Devnet infrastrucuture

## Run infrastructure with sample echo DApp

```
docker compose up
```

## Send input

```
INPUT=0x68656C6C6F206E6F6465 ;\
INPUT_BOX_ADDRESS=0x59b22D57D4f067708AB0c00552767405926dc768 ;\
DAPP_ADDRESS=0x70ac08179605AF2D9e75782b8DEcDD3c22aA4D0C ;\
cast send $INPUT_BOX_ADDRESS "addInput(address,bytes)(bytes32)" $DAPP_ADDRESS $INPUT --mnemonic "test test test test test test test test test test test junk" --rpc-url "http://localhost:8545"
```

## query input

Access `http://localhost:4000/graphql` and execute query
```
query{
  notices {edges{node{index,payload,input{payload}}}}
}
```