#!/bin/bash


# Run Anvil in the background
cmd="anvil --host 0.0.0.0 --dump-state anvil_state.json"
eval "${cmd}" &>/dev/null & disown;


# Check Anvil is ready
retries=0
max_retries=50
while [ $retries -lt $max_retries ]; do
  
    ready=$(curl -X POST -s -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' http://127.0.0.1:8545 | jq '.result') 

    if [ "$ready" == "true" ]; then
       echo "Anvil is ready"
       break
    else
       sleep 3
    fi
done


# Deploy contracts with Foundry
### DEPLOY CONTRACTS HERE ###

# TEMP : To help debug
# read -n 1 -s -r -p "Press any key to continue"

sleep 3
#Kill Anvil ( just exit )
exit 0