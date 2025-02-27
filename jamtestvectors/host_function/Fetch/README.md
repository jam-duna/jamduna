# Host Fetch  

## hostFetchOK  
1. Successfully read fetch segments from initial fetch segments.  
2. Write fetch segments into memory.  

## hostFetchOK_gt_wg  
The size to be wrote is larger than wg (24 in our case), so only wg bytes will be wrote into memory.  
1. Successfully read fetch segments from initial fetch segments.  
2. Write fetch segments into memory.  

## hostFetchOOB  
1. Successfully read fetch segments from initial fetch segments.  
2. Failed to write the fetch segments into memory due to a permission error.  

## hostFetchNONE  
1. Length of fetch segment is lower than omega_7.  