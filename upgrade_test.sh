#!/usr/bin/env bash
# Devnet addresses
#31 52.17 deploying "Bitmask" (tx: 0xfa22ef2f5ae2534abe530247f9a367f3533816c4ff7c10dac1a20910920a5421)...: deployed at 0xF5B2d8c81cDE4D6238bBf20D3D77DB37df13f735 with 165207 gas
#31 52.22 deploying "CartesiMathV2" (tx: 0xba4f9e8fbcfa3a11810ef97c8a0b1d0a46faadc5dd6aa0816f6d8c6818f4fee9)...: deployed at 0xB634F716BEd5Dd5A2b9a91C92474C499e50Cb27D with 490151 gas
#31 52.26 deploying "MerkleV2" (tx: 0x14b784b2ccdbcc1e9c1dab1744c59b30118187f9e5b864ae85b5e45831cc111e)...: deployed at 0x33436035441927Df1a73FE3AAC5906854632e53d with 1267230 gas
#31 52.30 deploying "UnrolledCordic" (tx: 0xc852e9857ead04afd70431c05f418ea6cf457e528fb58da419e650f7b761d3e3)...: deployed at 0x3F8FdcD1B0F421D817BF58C96b7C91B98100B450 with 395654 gas
#31 52.47 deploying "InputBox" (tx: 0xe5a9744422b4157a9a561ea223d244917a9340e8ab4e8e56f0a8bb46476b3716)...: deployed at 0x59b22D57D4f067708AB0c00552767405926dc768 with 260728 gas
#31 52.58 deploying "EtherPortal" (tx: 0xdf3c1045045626300076ca0cf1ceb34596395482283251b1b021baa4d76f28b1)...: deployed at 0xFfdbe43d4c855BF7e0f105c400A50857f53AB044 with 237926 gas
#31 52.68 deploying "ERC20Portal" (tx: 0xd8790d01fc1730f89f3bf9e5ec056c60c9e26676873fa0c1477abf2194b82d7a)...: deployed at 0x9C21AEb2093C32DDbC53eEF24B873BDCd1aDa1DB with 265834 gas
#31 52.78 deploying "ERC721Portal" (tx: 0xa19ec0e6e5bd7bbec5814abb64de5fd1181239fd9f47b18b15460bb34538327a)...: deployed at 0x237F8DD094C0e47f4236f12b4Fa01d6Dae89fb87 with 308450 gas
#31 52.87 deploying "ERC1155SinglePortal" (tx: 0xaef0ee062bcec244e0339482d071cca407078f531c899e90fceded05085f9c8d)...: deployed at 0x7CFB0193Ca87eB6e48056885E026552c3A941FC4 with 317542 gas
#31 52.96 deploying "ERC1155BatchPortal" (tx: 0x3c8c1b1ef6b831bfff0bf73c8d7acda176825f422f13e18832940c9337894cbb)...: deployed at 0xedB53860A6B52bbb7561Ad596416ee9965B055Aa with 370950 gas
#31 53.06 deploying "DAppAddressRelay" (tx: 0xdd6b0df4e37b2d003c616afb2d9155ed83772c40a55e51ad8b96fb30d75e4319)...: deployed at 0xF5DE34d6BbC0446E2a45719E718efEbaaE179daE with 175472 gas
#31 53.16 deploying "AuthorityFactory" (tx: 0xefaf8f2c733491166c08f9b9a2bf2bd1e885f0688757245aad501890d1e2410c)...: deployed at 0xf26a5b278C25D8D41A136d22Ad719EACEd9c3e63 with 756894 gas
#31 53.25 deploying "HistoryFactory" (tx: 0x884c0d73877c265c488863a0d23d950bb26b1dd8eb1d8ec0b0c034c5a8eda366)...: deployed at 0x1f158b5320BBf677FdA89F9a438df99BbE560A26 with 741828 gas
#31 53.35 deploying "AuthorityHistoryPairFactory" (tx: 0x65e82e708223a6a68478fd44b930b39fce020f8e82fe36d2ff31f83a54e829e3)...: deployed at 0x3890A047Cf9Af60731E80B2105362BbDCD70142D with 460428 gas
#31 53.49 deploying "CartesiDAppFactory" (tx: 0x22900111d1f6e43374bfcc460cb3b6975cf00d2ce0e4834ee64d9e487e6317a8)...: deployed at 0x7122cd1221C20892234186facfE8615e6743Ab02 with 1546312 gas

newHistory

>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
Usar ghcr image da PR para testar fix
docker pull ghcr.io/cartesi/rollups-node:pr-445
>>>>>>>>>>>>>>>>>>>>>>>>>>>>>

------

# Start node
docker compose -f ./build/compose-database.yaml -f ./build/compose-snapshot.yaml -f ./build/compose-node.yaml -f ./build/compose-devnet.yaml up

export CONTRACT_OWNER_ADDRESS=0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
export SALT=0x0000000000000000000000000000000000000000000000000000000000000000
export RPC_URL="http://localhost:8545"
export MNEMONIC="test test test test test test test test test test test junk"
export HISTORY_FACTORY_ADDRESS=0x1f158b5320BBf677FdA89F9a438df99BbE560A26

cast call \
    --trace --verbose \
    $HISTORY_FACTORY_ADDRESS \
    "calculateHistoryAddress(address,bytes32)(address)" \
    $CONTRACT_OWNER_ADDRESS \
    $SALT \
    --rpc-url "$RPC_URL"

Traces:
  [7710] 0x1f158b5320BBf677FdA89F9a438df99BbE560A26::calculateHistoryAddress(0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266, 0x0000000000000000000000000000000000000000000000000000000000000000)
    └─ ← 0x000000000000000000000000ce5f9be3a409dfbff2ecea8a1e861b66d7ec927c

cast call \
    --trace --verbose \
    "$HISTORY_FACTORY_ADDRESS" \
    "newHistory(address,bytes32)(History)" \
    "$CONTRACT_OWNER_ADDRESS" \
    "$SALT" \
    --rpc-url "$RPC_URL"

cast send \
    --json \
    --mnemonic "$MNEMONIC" \
    $HISTORY_FACTORY_ADDRESS \
    "newHistory(address,bytes32)(History)" \
    $CONTRACT_OWNER_ADDRESS \
    $SALT \
    --rpc-url "$RPC_URL"

{"transactionHash":"0x42608c2977f41d0feb7ddd0022aa73e3bfa6bb8daf16f42be5583a994f81e25d","transactionIndex":"0x0","blockHash":"0x21acf93184ac9478873a406fdfef1f9222dfd0eb1b9f27c2d4181c1233c80a79","blockNumber":"0x7f","from":"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266","to":"0x1f158b5320bbf677fda89f9a438df99bbe560a26","cumulativeGasUsed":"0x70553","gasUsed":"0x70553","contractAddress":null,"logs":[{"address":"0xce5f9be3a409dfbff2ecea8a1e861b66d7ec927c","topics":["0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0","0x0000000000000000000000000000000000000000000000000000000000000000","0x0000000000000000000000001f158b5320bbf677fda89f9a438df99bbe560a26"],"data":"0x","blockHash":"0x21acf93184ac9478873a406fdfef1f9222dfd0eb1b9f27c2d4181c1233c80a79","blockNumber":"0x7f","transactionHash":"0x42608c2977f41d0feb7ddd0022aa73e3bfa6bb8daf16f42be5583a994f81e25d","transactionIndex":"0x0","logIndex":"0x0","removed":false},{"address":"0xce5f9be3a409dfbff2ecea8a1e861b66d7ec927c","topics":["0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0","0x0000000000000000000000001f158b5320bbf677fda89f9a438df99bbe560a26","0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266"],"data":"0x","blockHash":"0x21acf93184ac9478873a406fdfef1f9222dfd0eb1b9f27c2d4181c1233c80a79","blockNumber":"0x7f","transactionHash":"0x42608c2977f41d0feb7ddd0022aa73e3bfa6bb8daf16f42be5583a994f81e25d","transactionIndex":"0x0","logIndex":"0x1","removed":false},{"address":"0x1f158b5320bbf677fda89f9a438df99bbe560a26","topics":["0x5b0ee1fb14fdb4a34ba9f0dda6790b059be11d8b3a954905dbad3c025c05c9c6"],"data":"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266000000000000000000000000ce5f9be3a409dfbff2ecea8a1e861b66d7ec927c","blockHash":"0x21acf93184ac9478873a406fdfef1f9222dfd0eb1b9f27c2d4181c1233c80a79","blockNumber":"0x7f","transactionHash":"0x42608c2977f41d0feb7ddd0022aa73e3bfa6bb8daf16f42be5583a994f81e25d","transactionIndex":"0x0","logIndex":"0x2","removed":false}],"status":"0x1","logsBloom":"0x00000000000400000000000000000000000000000000000000800000000000000000000010000000000000001000000000000000000000000000000000000000000000000040000000000000002000000001000000000000000000000000000000000000020000000000000100000800040000000080000000000000000000408000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000100000000000000000000000000000000000200000000040000000000800002000000000000000000020000000000000000000000000000000000000000000000000000000000000000000","type":"0x2","effectiveGasPrice":"0xb2d06073","deposit_nonce":null}

docker compose -f ./build/compose-database.yaml -f ./build/compose-snapshot.yaml -f ./build/compose-node.yaml down

# Restart node (overrides history)
docker compose -f ./build/compose-database.yaml -f ./build/compose-snapshot.yaml -f ./build/compose-devnet.yaml -f ./build/compose-node-1_5.yaml  up

# Steps (markdown)

0. set epoch duration to 10 second
1. add an input, wait for epoch_duration and save anvil state
2. Set new history
3. Should we change block time to mimic 10 seconds? Perhaps 10 blocks with block-time 1

```
# Produces a new block every 10 seconds
anvil --block-time 10
```
4. start node on 1.5 with the new history
5. input should be processed and the epoch should be closed similarly with the same outputs being created
6. Should we show how to update config from epoch duration to block length

# Set no-mining is not necessary!!!!

##################------------------------
Implementation

# Generate devnet and deploy application
go run ./cmd/gen-devnet/
# Save anvil state

# Start env vars needed to run devnet
docker compose \
    -f ./build/compose-devnet.yaml

# Start v.1.4.0
docker compose \
    -f ./build/compose-database.yaml \
    -f ./build/compose-snapshot.yaml \
    -f ./build/compose-node.yaml \
    up

# Send inputs
# Advance time
# Gather application state info

# Save anvil state
# Keep anvil running

# Shutdown v.1.4.0
docker compose \
    -f ./build/compose-database.yaml \
    -f ./build/compose-snapshot.yaml \
    -f ./build/compose-node.yaml \
down

# Deploy and set new history
# save anvil state

# Start env vars needed to run devnet on new history
docker compose \
    -f ./build/compose-devnet.yaml

# Start v.1.5.0
docker compose \
    -f ./build/compose-database.yaml \
    -f ./build/compose-snapshot.yaml \
    -f ./build/compose-node.yaml \
    up

# Advance time
# Gather state info

# Shutdown v.1.5.0
docker compose \
    -f ./build/compose-database.yaml \
    -f ./build/compose-snapshot.yaml \
    -f ./build/compose-node.yaml \
down


# Compare state

