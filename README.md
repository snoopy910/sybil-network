This project is to create a zk-rollup for proof-of-uniqueness. This is a system where users can register an Ethereum address in a Sybil resistant way. This register can be used for many things such as doing an airdrop that can’t be cheated and even things like voting systems. Furthermore, with this register, users can also do things like make a proof that they own an Ethereum address that is on the register without revealing which one, which allows things like private voting.


The system allows account to deposit TON and 'stake' part of that TON on different accounts. This results in the data of a weighted graph. Next an algorithm is run on this weighted graph to compute a 'score' for each account. This algorithm is computationally expensive so is encoded as a circuit and run off chain and then a proof of correctness is sent to the L1 contract. 

The way the 'scoring algorithm' works is by starting with a set of subsets of the nodes (usually these are the subsets that are smaller than a certain threshold). Then to compute the score for a node, we look at each of these subsets that contains the node and compute the sum of the weights of the links that are leaving this subset and divide it by the number of elements in the subset. The score is then defined to be minimum of these values.

Example:


![graph](https://github.com/tokamak-network/proof-of-uniqueness/blob/main/graph-example.png?raw=true)


In this example, let us suppose the set of subsets we are using is all subsets with three or less nodes. Then, to compute the score for node 5, we look at all subsets with three or less elements, that contain node 5:

```{5},{0,5},{1,5},{2,5},{3,5},{4,5},{5,6},{0,1,5},{0,2,5},{0,3,5},{0,4,5},{0,6,5},{1,2,5},{1,3,5},{1,4,5},{1,5,6},{2,3,5},{2,4,5},{2,5,6},{3,4,5},{3,5,6},{4,5,6}```

Then for each of these subsets, we compute the sum of the weights of the links leaving the subset. For example, for the subset ```{1,3,5}``` the sum of the links leaving it is ```7+1+1+1+2=12```. Then we divide this by the number of elements in the subset we get ```12/3=4```. If we do this for each subset we get a list of values, and then if we take the minimum of this list of values we get the score for node 5.


This algorithm is implemented in the file [scoring_algorithm.circom](https://github.com/tokamak-network/proof-of-uniqueness/blob/main/scoring_algorithm.circom). It is implemented as a circom circuit so that the rollup sequencer can compute the scores on chain and send a proof to convince the L1 contract that the score have been computed correctly.

To test this code first install circom (https://docs.circom.io/getting-started/installation/)

https://docs.circom.io/getting-started/installation/
https://docs.circom.io/getting-started/installation/

```scoring_algorithm.circom```. To test this code
(circom and snarkjs are both required for this test), first run 

circom scoring_circuit.circom --r1cs --wasm --sym --c

This compiles the circuit and generates the constraints for the circuit. We then need to provide the data of a weighted graph for the circuit to run on, an example is given in the file test_input.json.

Then next step is to calculate values for all wires from input wires (using js program generated when circuit was compiled):

node generate_witness.js scoring_circuit.wasm input.json witness.wtns

To create a proof, circom requires a trusted setup. This is done using the powers of tau ceremony as follows:


snarkjs powersoftau new bn128 12 pot12_0000.ptau -v
snarkjs powersoftau contribute pot12_0000.ptau pot12_0001.ptau --name="First contribution" -v
snarkjs powersoftau prepare phase2 pot12_0001.ptau pot12_final.ptau -v
snarkjs plonk setup scoring_circuit.r1cs pot12_final.ptau scoring_circuit_final.zkey 

Next we export the verification key: snarkjs zkey export verificationkey scoring_circuit_final.zkey verification_key.json 

And now finally we create a PLonk proof for the witness: snarkjs plonk prove scoring_circuit_final.zkey witness.wtns proof.json public.json  


To verify the proof run the command: snarkjs plonk verify verification_key.json public.json proof.json   
