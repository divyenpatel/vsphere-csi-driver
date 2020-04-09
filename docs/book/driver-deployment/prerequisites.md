<!-- markdownlint-disable MD033 -->
# vSphere CSI Driver - Prerequisites

## Compatible vSphere and ESXi versions

## vSphere Roles and Privileges

The vSphere user for CSI driver requires set of privileges to perform Cloud Native Storage operations.

To know how to create and assign role, refer the [vSphere Security documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.security.doc/GUID-41E5E52E-A95B-4E81-9724-6AD6800BEF78.html).

Following roles needs to created with sets of permissions.

| Role                    | Privileges for the role | Required on                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
|-------------------------|-------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| CNS-DATASTORE           | ![ROLE-CNS-DATASTORE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/ROLE-CNS-DATASTORE.png)<br><pre lang="bash">govc role.ls CNS-DATASTORE<br>Datastore.FileManagement<br>System.Anonymous<br>System.Read<br>System.View<br></pre>| Shared datastores where persistent volumes needs to be provisioned..                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| CNS-HOST-CONFIG-STORAGE | ![ROLE-CNS-HOST-CONFIG-STORAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/ROLE-CNS-HOST-CONFIG-STORAGE.png)<br><pre lang="bash">% govc role.ls CNS-HOST-CONFIG-STORAGE<br>Host.Config.Storage<br>System.Anonymous<br>System.Read<br>System.View<br></pre>| Required on vSAN file service enabled vSAN cluster. Required for file volume only.                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| CNS-VM                  | ![ROLE-CNS-VM](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/ROLE-CNS-VM.png)<br><pre lang="bash">% govc role.ls CNS-VM<br>System.Anonymous<br>System.Read<br>System.View<br>VirtualMachine.Config.AddExistingDisk<br>VirtualMachine.Config.AddRemoveDevice<br></pre>| All node VMs                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| CNS-SEARCH-AND-SPBM     | ![ROLE-CNS-SEARCH-AND-SPBM](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/ROLE-CNS-SEARCH-AND-SPBM.png)<br><pre lang="bash">% govc role.ls CNS-SEARCH-AND-SPBM<br>Cns.Searchable<br>StorageProfile.View<br>System.Anonymous<br>System.Read<br>System.View<br></pre>| Root vCenter Server                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ReadOnly                | This role is already available in the vCenter.<br><pre lang="bash">% govc role.ls ReadOnly<br>System.Anonymous<br>System.Read<br></pre>| Users with the Read Only role for an object are allowed to view the state of the object and details about the object.<br><br>For example, users,with this role can find the shared datastore accessible to all node VMs.<br><br>For,zone and topology-aware environments, all ancestors of node VMs, such,as a host, cluster, and data center must have the Read-only role set for,the vSphere user configured to use the CSI driver and CPI.<br>This is required to allow reading tags and categories to prepare the nodes' topology. |

Roles needs to be assigned to the vSphere objects that participate in the Cloud Native Storage environment.

To understand roles assigment to vSphere objects, consider we have following vSphere inventory.

```bash
sc2-rdops-vm06-dhcp-215-129.eng.vmware.com (vCenter Server)
|
|- datacenter (Data Center)
    |
    |-vSAN-cluster (cluster)
      |
      |-10.192.209.1 (ESXi Host)
      | |
      | |-k8s-master (node-vm)
      |
      |-10.192.211.250 (ESXi Host)
      | |
      | |-k8s-node1 (node-vm)
      |
      |-10.192.217.166 (ESXi Host)
      | |
      | |-k8s-node2 (node-vm)
      | |
      |-10.192.218.26 (ESXi Host)
      | |
      | |-k8s-node3 (node-vm)
```

Consider each host has following shared datastores along with some local VMFS datastores.

- shared-vmfs
- shared-nfs
- vsanDatastore

Considering above inventory, roles should be assigned as specified below.

| Role  | Usage  |
|---|---|
| Read-Only | ![READ-ONLY-USAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/READ-ONLY-USAGE.png)  |
| CNS-HOST-CONFIG-STORAGE | ![HOST-CONFIG-STORAGE-USAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/HOST-CONFIG-STORAGE-USAGE.png)   |
| CNS-DATASTORE | ![CNS-DATASTORE-USAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/CNS-DATASTORE-USAGE.png)  |
| CNS-VM | ![CNS-VM-USAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/CNS-VM-USAGE.png)  |
| CNS-SEARCH-AND-SPBM | ![CNS-SEARCH-AND-SPBM-USAGE](https://raw.githubusercontent.com/kubernetes-sigs/vsphere-csi-driver/master/docs/images/CNS-SEARCH-AND-SPBM-USAGE.png)  |

## Setting up the management network

## Virtual Machine Configuration

## vSphere Cloud Provider Interface
