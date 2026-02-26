#!/bin/sh

for kv in \
	"CARTESI_CONTRACTS_INPUT_BOX_ADDRESS                       InputBox" \
	"CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS               AuthorityFactory" \
	"CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS             ApplicationFactory" \
	"CARTESI_CONTRACTS_SELF_HOSTED_APPLICATION_FACTORY_ADDRESS SelfHostedApplicationFactory" \
	"CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS                DaveAppFactory";
do
	mkey=$(echo $kv | cut -d' ' -f1)
	jkey=$(echo $kv | cut -d' ' -f2)

	echo -e "\t@echo export $mkey=$(jq ".[\"$jkey\"]" < deployment.json)"
done
