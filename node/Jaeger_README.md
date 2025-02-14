## **Jaeger Web Tutorial**  

This tutorial explains how to configure the fields on the left side of the web interface.  

### **Service Field**  
Select **JAM**.  

### **Operation**  
Format: `"[N%d] function name"`, e.g., `[N3] broadcastWorkpackage`.  
You can search for a specific operation by defining its **NodeID** and function name.  

### **Tags**  
This field is used to search for tags within a specific trace.  
Currently supported tags:  
- **WorkpackageHash**  
- **BlockHash**  

To search for a specific work package, enter:  
`WorkpackageHash=8380..870f` *(Copy WorkpackageHash directly from the command line output.)*  

To search for a specific block, enter:  
`BlockHash=8380..870f` *(Copy BlockHash directly from the command line output.)*  

### **Notes**  
- **Remember to set "Lookback"**, otherwise, you won’t see any results.  
- **Remember to adjust "Limit Results"** (should be ≤ 1500).  

---

### **Tracked Functions**  
- **broadcastWorkpackage**  
  ├── ShareWorkPackage  
  └── executeWorkPackage  

- **SendWorkReportDistribution**  

- **SendSegmentShardRequest**  

- **SendFullShardRequest**  

- **SendBundleShardRequest**  

- **AddGuaranteeToPool**  

- **AddAssuranceToPool**  

- **MakeBlock**  
  └── ApplyStateTransitionFromBlock  

- **processBlockAnnouncement**  
  └── SendBlockRequest  
