. as $root | .contracts |
reduce to_entries[] as $c (
    {"name": $root.name, "ChainId": $root.chainId | tonumber};
    . + {($c.key): ($c.value.address)}
)
