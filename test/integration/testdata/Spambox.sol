// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
pragma solidity ^0.8.30;

interface IInputBoxForSpambox {
    function addInput(address app, bytes calldata payload) external returns (bytes32);
}

contract Spambox {
    IInputBoxForSpambox public immutable inputBox;

    constructor(address inputBox_) {
        inputBox = IInputBoxForSpambox(inputBox_);
    }

    function spam(address app, uint256 count) external {
        for (uint256 i = 0; i < count; i++) {
            inputBox.addInput(app, abi.encodePacked("SPAM-", i));
        }
    }
}
