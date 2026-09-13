// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
pragma solidity ^0.8.30;

interface IInputBoxForInputRelay {
    function addInput(address app, bytes calldata payload) external returns (bytes32);
}

contract InputRelay {
    IInputBoxForInputRelay public immutable inputBox;

    constructor(address inputBox_) {
        inputBox = IInputBoxForInputRelay(inputBox_);
    }

    function addInput(address app, bytes calldata payload) external returns (bytes32) {
        return inputBox.addInput(app, payload);
    }

    fallback() external payable {}
}
