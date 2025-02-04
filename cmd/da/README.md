# Data Availability
## 24 Node Setup

There are two different methods to run 24 nodes depending on the use case.

### Method 1: Local Testing
This method utilizes `make network_local` to start the network locally. The following three parameters are essential:
- `validatorindex`: Specifies the node index.
- `port`: Defines the port number.
- `metadata`: Determines the entity used to connect to the node.

Each node is connected via different local ports.

### Method 2: Multi-VM Network Communication
This method involves actual network communication between different virtual machines (VMs). Several networking considerations need attention:
- Ensure that packet validation is disabled, which can be checked using:
  ```sh
  sudo tcpdump -i ens4 port 9000 -vvv
  ```
- Ensure sufficient cache allocation using the following commands:
  ```sh
  ulimit -n 65535
  sysctl -w net.core.rmem_max=26214400
  sysctl -w net.core.wmem_max=26214400
  ```

In this setup, all nodes communicate via port `9000`. The target VM is specified using the `-hp` flag. Since VMs are named as `jam-0` to `jam-X`, the prefix `jam-` should be used. If the naming convention changes in the future, adjustments will be required.

#### Node Behavior
1. **Node 0** performs erasure coding on randomly sized data.
2. The encoded data is distributed across different nodes.
3. A hash is sent to the last node, which is then notified to start fetching and reconstructing the data.
4. The last node decodes the collected data and notifies **Node 0** to repeat the process.

#### Running the Nodes
The following commands are required to run the nodes. In the future, these can be incorporated into a Makefile:
```sh
cdj
export GOFLAGS="-tags=small"
git checkout da
git reset --hard && git fetch origin && git reset --hard origin/$(git rev-parse --abbrev-ref HEAD)
git fetch
git pull
cd cmd/da
make kill
make da
```
The fourth command:
```sh
git reset --hard && git fetch origin && git reset --hard origin/$(git rev-parse --abbrev-ref HEAD)
```
is used to reset the head. If not needed, it can be removed.

To execute `da`, use the following command:
```sh
./da -port 9000 -metadata da- -hp jam- -validatorindex 0
```
It might be possible to automate the assignment of the last numerical value.

---

## Integrating DA Results into the Main Node Package
Currently, a basic method is used. Future modifications should distribute and reconstruct data based on actual workpackage bundles (`b`) and segments (`s`).

### Steps to Implement:
1. Generate random **Workpackages** and **Segments**, then hash them. (Consider generating meaningful Workpackages and Segments.)
2. Process Workpackages and Segments into `b^♣`, `s^♣`, and the **erasure root**. This step aligns with `NewAvailabilitySpecifier()`.
3. Distribute the processed data.
4. Ensure that at least **2/3 of nodes confirm assurance**. Once this threshold is reached, the last node initiates **reconstruction**.
5. Each time the last node collects data, it verifies its correctness. If valid, the data is used for decoding.
6. Decode the data and verify whether its hash matches the original data hash.

Currently, **Node 0 generates and distributes the data**, and **the last node reconstructs it**. However, future modifications might involve different nodes taking turns in both generating and reconstructing the data.