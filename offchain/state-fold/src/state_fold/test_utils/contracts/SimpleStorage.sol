// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.7.0;

contract SimpleStorage {

    event ValueChanged(
      address indexed author,
      address indexed oldAuthor,
      uint256 indexed n,
      string oldValue,
      string newValue
    );

    address public lastSender;
    string _value;
    uint256 _n;

    constructor(string memory value) {
        emit ValueChanged(msg.sender, address(0), _n, _value, value);
        lastSender = msg.sender;
        _value = value;
    }

    function getValue() view public returns (string memory) {
        return _value;
    }

    function getValues() view public returns (string memory, address) {
        return (_value, lastSender);
    }

    function setValue(string memory value) public {
        _n++;
        emit ValueChanged(msg.sender, lastSender, _n, _value, value);
        _value = value;
        lastSender = msg.sender;
    }
}
