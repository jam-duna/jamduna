#!/bin/bash

# Auto-generated script to run all trace tests
# Generated on Wed Nov 26 16:10:05 EST 2025

mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy
mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light
mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages
mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light
mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage
mkdir -p /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light

# Processing fuzzy directory
echo "Running test for fuzzy/00000001.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000001.log.gz
echo "Running test for fuzzy/00000002.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000002.log.gz
echo "Running test for fuzzy/00000003.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000003.log.gz
echo "Running test for fuzzy/00000004.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000004.log.gz
echo "Running test for fuzzy/00000005.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000005.log.gz
echo "Running test for fuzzy/00000006.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000006.log.gz
echo "Running test for fuzzy/00000007.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000007.log.gz
echo "Running test for fuzzy/00000008.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000008.log.gz
echo "Running test for fuzzy/00000009.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000009.log.gz
echo "Running test for fuzzy/00000010.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000010.log.gz
echo "Running test for fuzzy/00000011.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000011.log.gz
echo "Running test for fuzzy/00000012.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000012.log.gz
echo "Running test for fuzzy/00000013.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000013.log.gz
echo "Running test for fuzzy/00000014.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000014.log.gz
echo "Running test for fuzzy/00000015.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000015.log.gz
echo "Running test for fuzzy/00000016.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000016.log.gz
echo "Running test for fuzzy/00000017.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000017.log.gz
echo "Running test for fuzzy/00000018.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000018.log.gz
echo "Running test for fuzzy/00000019.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000019.log.gz
echo "Running test for fuzzy/00000020.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000020.log.gz
echo "Running test for fuzzy/00000021.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000021.log.gz
echo "Running test for fuzzy/00000022.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000022.log.gz
echo "Running test for fuzzy/00000023.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000023.log.gz
echo "Running test for fuzzy/00000024.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000024.log.gz
echo "Running test for fuzzy/00000025.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000025.log.gz
echo "Running test for fuzzy/00000026.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000026.log.gz
echo "Running test for fuzzy/00000027.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000027.log.gz
echo "Running test for fuzzy/00000028.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000028.log.gz
echo "Running test for fuzzy/00000029.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000029.log.gz
echo "Running test for fuzzy/00000030.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000030.log.gz
echo "Running test for fuzzy/00000031.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000031.log.gz
echo "Running test for fuzzy/00000032.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000032.log.gz
echo "Running test for fuzzy/00000033.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000033.log.gz
echo "Running test for fuzzy/00000034.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000034.log.gz
echo "Running test for fuzzy/00000035.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000035.log.gz
echo "Running test for fuzzy/00000036.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000036.log.gz
echo "Running test for fuzzy/00000037.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000037.log.gz
echo "Running test for fuzzy/00000038.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000038.log.gz
echo "Running test for fuzzy/00000039.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000039.log.gz
echo "Running test for fuzzy/00000040.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000040.log.gz
echo "Running test for fuzzy/00000041.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000041.log.gz
echo "Running test for fuzzy/00000042.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000042.log.gz
echo "Running test for fuzzy/00000043.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000043.log.gz
echo "Running test for fuzzy/00000044.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000044.log.gz
echo "Running test for fuzzy/00000045.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000045.log.gz
echo "Running test for fuzzy/00000046.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000046.log.gz
echo "Running test for fuzzy/00000047.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000047.log.gz
echo "Running test for fuzzy/00000048.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000048.log.gz
echo "Running test for fuzzy/00000049.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000049.log.gz
echo "Running test for fuzzy/00000050.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000050.log.gz
echo "Running test for fuzzy/00000051.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000051.log.gz
echo "Running test for fuzzy/00000052.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000052.log.gz
echo "Running test for fuzzy/00000053.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000053.log.gz
echo "Running test for fuzzy/00000054.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000054.log.gz
echo "Running test for fuzzy/00000055.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000055.log.gz
echo "Running test for fuzzy/00000056.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000056.log.gz
echo "Running test for fuzzy/00000057.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000057.log.gz
echo "Running test for fuzzy/00000058.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000058.log.gz
echo "Running test for fuzzy/00000059.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000059.log.gz
echo "Running test for fuzzy/00000060.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000060.log.gz
echo "Running test for fuzzy/00000061.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000061.log.gz
echo "Running test for fuzzy/00000062.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000062.log.gz
echo "Running test for fuzzy/00000063.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000063.log.gz
echo "Running test for fuzzy/00000064.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000064.log.gz
echo "Running test for fuzzy/00000065.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000065.log.gz
echo "Running test for fuzzy/00000066.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000066.log.gz
echo "Running test for fuzzy/00000067.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000067.log.gz
echo "Running test for fuzzy/00000068.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000068.log.gz
echo "Running test for fuzzy/00000069.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000069.log.gz
echo "Running test for fuzzy/00000070.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000070.log.gz
echo "Running test for fuzzy/00000071.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000071.log.gz
echo "Running test for fuzzy/00000072.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000072.log.gz
echo "Running test for fuzzy/00000073.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000073.log.gz
echo "Running test for fuzzy/00000074.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000074.log.gz
echo "Running test for fuzzy/00000075.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000075.log.gz
echo "Running test for fuzzy/00000076.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000076.log.gz
echo "Running test for fuzzy/00000077.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000077.log.gz
echo "Running test for fuzzy/00000078.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000078.log.gz
echo "Running test for fuzzy/00000079.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000079.log.gz
echo "Running test for fuzzy/00000080.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000080.log.gz
echo "Running test for fuzzy/00000081.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000081.log.gz
echo "Running test for fuzzy/00000082.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000082.log.gz
echo "Running test for fuzzy/00000083.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000083.log.gz
echo "Running test for fuzzy/00000084.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000084.log.gz
echo "Running test for fuzzy/00000085.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000085.log.gz
echo "Running test for fuzzy/00000086.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000086.log.gz
echo "Running test for fuzzy/00000087.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000087.log.gz
echo "Running test for fuzzy/00000088.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000088.log.gz
echo "Running test for fuzzy/00000089.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000089.log.gz
echo "Running test for fuzzy/00000090.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000090.log.gz
echo "Running test for fuzzy/00000091.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000091.log.gz
echo "Running test for fuzzy/00000092.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000092.log.gz
echo "Running test for fuzzy/00000093.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000093.log.gz
echo "Running test for fuzzy/00000094.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000094.log.gz
echo "Running test for fuzzy/00000095.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000095.log.gz
echo "Running test for fuzzy/00000096.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000096.log.gz
echo "Running test for fuzzy/00000097.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000097.log.gz
echo "Running test for fuzzy/00000098.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000098.log.gz
echo "Running test for fuzzy/00000099.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000099.log.gz
echo "Running test for fuzzy/00000100.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000100.log.gz
echo "Running test for fuzzy/00000101.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000101.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000101.log.gz
echo "Running test for fuzzy/00000102.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000102.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000102.log.gz
echo "Running test for fuzzy/00000103.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000103.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000103.log.gz
echo "Running test for fuzzy/00000104.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000104.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000104.log.gz
echo "Running test for fuzzy/00000105.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000105.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000105.log.gz
echo "Running test for fuzzy/00000106.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000106.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000106.log.gz
echo "Running test for fuzzy/00000107.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000107.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000107.log.gz
echo "Running test for fuzzy/00000108.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000108.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000108.log.gz
echo "Running test for fuzzy/00000109.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000109.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000109.log.gz
echo "Running test for fuzzy/00000110.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000110.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000110.log.gz
echo "Running test for fuzzy/00000111.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000111.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000111.log.gz
echo "Running test for fuzzy/00000112.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000112.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000112.log.gz
echo "Running test for fuzzy/00000113.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000113.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000113.log.gz
echo "Running test for fuzzy/00000114.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000114.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000114.log.gz
echo "Running test for fuzzy/00000115.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000115.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000115.log.gz
echo "Running test for fuzzy/00000116.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000116.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000116.log.gz
echo "Running test for fuzzy/00000117.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000117.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000117.log.gz
echo "Running test for fuzzy/00000118.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000118.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000118.log.gz
echo "Running test for fuzzy/00000119.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000119.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000119.log.gz
echo "Running test for fuzzy/00000120.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000120.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000120.log.gz
echo "Running test for fuzzy/00000121.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000121.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000121.log.gz
echo "Running test for fuzzy/00000122.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000122.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000122.log.gz
echo "Running test for fuzzy/00000123.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000123.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000123.log.gz
echo "Running test for fuzzy/00000124.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000124.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000124.log.gz
echo "Running test for fuzzy/00000125.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000125.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000125.log.gz
echo "Running test for fuzzy/00000126.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000126.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000126.log.gz
echo "Running test for fuzzy/00000127.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000127.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000127.log.gz
echo "Running test for fuzzy/00000128.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000128.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000128.log.gz
echo "Running test for fuzzy/00000129.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000129.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000129.log.gz
echo "Running test for fuzzy/00000130.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000130.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000130.log.gz
echo "Running test for fuzzy/00000131.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000131.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000131.log.gz
echo "Running test for fuzzy/00000132.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000132.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000132.log.gz
echo "Running test for fuzzy/00000133.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000133.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000133.log.gz
echo "Running test for fuzzy/00000134.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000134.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000134.log.gz
echo "Running test for fuzzy/00000135.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000135.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000135.log.gz
echo "Running test for fuzzy/00000136.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000136.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000136.log.gz
echo "Running test for fuzzy/00000137.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000137.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000137.log.gz
echo "Running test for fuzzy/00000138.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000138.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000138.log.gz
echo "Running test for fuzzy/00000139.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000139.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000139.log.gz
echo "Running test for fuzzy/00000140.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000140.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000140.log.gz
echo "Running test for fuzzy/00000141.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000141.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000141.log.gz
echo "Running test for fuzzy/00000142.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000142.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000142.log.gz
echo "Running test for fuzzy/00000143.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000143.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000143.log.gz
echo "Running test for fuzzy/00000144.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000144.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000144.log.gz
echo "Running test for fuzzy/00000145.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000145.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000145.log.gz
echo "Running test for fuzzy/00000146.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000146.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000146.log.gz
echo "Running test for fuzzy/00000147.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000147.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000147.log.gz
echo "Running test for fuzzy/00000148.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000148.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000148.log.gz
echo "Running test for fuzzy/00000149.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000149.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000149.log.gz
echo "Running test for fuzzy/00000150.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000150.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000150.log.gz
echo "Running test for fuzzy/00000151.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000151.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000151.log.gz
echo "Running test for fuzzy/00000152.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000152.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000152.log.gz
echo "Running test for fuzzy/00000153.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000153.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000153.log.gz
echo "Running test for fuzzy/00000154.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000154.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000154.log.gz
echo "Running test for fuzzy/00000155.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000155.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000155.log.gz
echo "Running test for fuzzy/00000156.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000156.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000156.log.gz
echo "Running test for fuzzy/00000157.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000157.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000157.log.gz
echo "Running test for fuzzy/00000158.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000158.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000158.log.gz
echo "Running test for fuzzy/00000159.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000159.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000159.log.gz
echo "Running test for fuzzy/00000160.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000160.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000160.log.gz
echo "Running test for fuzzy/00000161.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000161.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000161.log.gz
echo "Running test for fuzzy/00000162.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000162.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000162.log.gz
echo "Running test for fuzzy/00000163.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000163.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000163.log.gz
echo "Running test for fuzzy/00000164.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000164.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000164.log.gz
echo "Running test for fuzzy/00000165.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000165.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000165.log.gz
echo "Running test for fuzzy/00000166.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000166.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000166.log.gz
echo "Running test for fuzzy/00000167.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000167.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000167.log.gz
echo "Running test for fuzzy/00000168.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000168.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000168.log.gz
echo "Running test for fuzzy/00000169.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000169.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000169.log.gz
echo "Running test for fuzzy/00000170.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000170.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000170.log.gz
echo "Running test for fuzzy/00000171.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000171.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000171.log.gz
echo "Running test for fuzzy/00000172.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000172.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000172.log.gz
echo "Running test for fuzzy/00000173.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000173.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000173.log.gz
echo "Running test for fuzzy/00000174.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000174.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000174.log.gz
echo "Running test for fuzzy/00000175.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000175.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000175.log.gz
echo "Running test for fuzzy/00000176.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000176.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000176.log.gz
echo "Running test for fuzzy/00000177.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000177.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000177.log.gz
echo "Running test for fuzzy/00000178.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000178.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000178.log.gz
echo "Running test for fuzzy/00000179.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000179.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000179.log.gz
echo "Running test for fuzzy/00000180.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000180.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000180.log.gz
echo "Running test for fuzzy/00000181.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000181.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000181.log.gz
echo "Running test for fuzzy/00000182.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000182.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000182.log.gz
echo "Running test for fuzzy/00000183.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000183.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000183.log.gz
echo "Running test for fuzzy/00000184.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000184.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000184.log.gz
echo "Running test for fuzzy/00000185.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000185.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000185.log.gz
echo "Running test for fuzzy/00000186.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000186.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000186.log.gz
echo "Running test for fuzzy/00000187.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000187.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000187.log.gz
echo "Running test for fuzzy/00000188.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000188.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000188.log.gz
echo "Running test for fuzzy/00000189.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000189.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000189.log.gz
echo "Running test for fuzzy/00000190.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000190.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000190.log.gz
echo "Running test for fuzzy/00000191.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000191.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000191.log.gz
echo "Running test for fuzzy/00000192.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000192.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000192.log.gz
echo "Running test for fuzzy/00000193.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000193.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000193.log.gz
echo "Running test for fuzzy/00000194.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000194.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000194.log.gz
echo "Running test for fuzzy/00000195.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000195.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000195.log.gz
echo "Running test for fuzzy/00000196.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000196.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000196.log.gz
echo "Running test for fuzzy/00000197.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000197.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000197.log.gz
echo "Running test for fuzzy/00000198.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000198.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000198.log.gz
echo "Running test for fuzzy/00000199.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000199.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000199.log.gz
echo "Running test for fuzzy/00000200.bin..."
go test -run="TestTracesGoInterpreter/fuzzy/00000200.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy/00000200.log.gz

# Processing fuzzy_light directory
echo "Running test for fuzzy_light/00000001.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000001.log.gz
echo "Running test for fuzzy_light/00000002.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000002.log.gz
echo "Running test for fuzzy_light/00000003.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000003.log.gz
echo "Running test for fuzzy_light/00000004.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000004.log.gz
echo "Running test for fuzzy_light/00000005.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000005.log.gz
echo "Running test for fuzzy_light/00000006.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000006.log.gz
echo "Running test for fuzzy_light/00000007.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000007.log.gz
echo "Running test for fuzzy_light/00000008.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000008.log.gz
echo "Running test for fuzzy_light/00000009.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000009.log.gz
echo "Running test for fuzzy_light/00000010.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000010.log.gz
echo "Running test for fuzzy_light/00000011.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000011.log.gz
echo "Running test for fuzzy_light/00000012.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000012.log.gz
echo "Running test for fuzzy_light/00000013.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000013.log.gz
echo "Running test for fuzzy_light/00000014.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000014.log.gz
echo "Running test for fuzzy_light/00000015.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000015.log.gz
echo "Running test for fuzzy_light/00000016.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000016.log.gz
echo "Running test for fuzzy_light/00000017.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000017.log.gz
echo "Running test for fuzzy_light/00000018.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000018.log.gz
echo "Running test for fuzzy_light/00000019.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000019.log.gz
echo "Running test for fuzzy_light/00000020.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000020.log.gz
echo "Running test for fuzzy_light/00000021.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000021.log.gz
echo "Running test for fuzzy_light/00000022.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000022.log.gz
echo "Running test for fuzzy_light/00000023.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000023.log.gz
echo "Running test for fuzzy_light/00000024.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000024.log.gz
echo "Running test for fuzzy_light/00000025.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000025.log.gz
echo "Running test for fuzzy_light/00000026.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000026.log.gz
echo "Running test for fuzzy_light/00000027.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000027.log.gz
echo "Running test for fuzzy_light/00000028.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000028.log.gz
echo "Running test for fuzzy_light/00000029.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000029.log.gz
echo "Running test for fuzzy_light/00000030.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000030.log.gz
echo "Running test for fuzzy_light/00000031.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000031.log.gz
echo "Running test for fuzzy_light/00000032.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000032.log.gz
echo "Running test for fuzzy_light/00000033.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000033.log.gz
echo "Running test for fuzzy_light/00000034.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000034.log.gz
echo "Running test for fuzzy_light/00000035.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000035.log.gz
echo "Running test for fuzzy_light/00000036.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000036.log.gz
echo "Running test for fuzzy_light/00000037.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000037.log.gz
echo "Running test for fuzzy_light/00000038.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000038.log.gz
echo "Running test for fuzzy_light/00000039.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000039.log.gz
echo "Running test for fuzzy_light/00000040.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000040.log.gz
echo "Running test for fuzzy_light/00000041.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000041.log.gz
echo "Running test for fuzzy_light/00000042.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000042.log.gz
echo "Running test for fuzzy_light/00000043.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000043.log.gz
echo "Running test for fuzzy_light/00000044.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000044.log.gz
echo "Running test for fuzzy_light/00000045.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000045.log.gz
echo "Running test for fuzzy_light/00000046.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000046.log.gz
echo "Running test for fuzzy_light/00000047.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000047.log.gz
echo "Running test for fuzzy_light/00000048.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000048.log.gz
echo "Running test for fuzzy_light/00000049.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000049.log.gz
echo "Running test for fuzzy_light/00000050.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000050.log.gz
echo "Running test for fuzzy_light/00000051.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000051.log.gz
echo "Running test for fuzzy_light/00000052.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000052.log.gz
echo "Running test for fuzzy_light/00000053.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000053.log.gz
echo "Running test for fuzzy_light/00000054.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000054.log.gz
echo "Running test for fuzzy_light/00000055.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000055.log.gz
echo "Running test for fuzzy_light/00000056.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000056.log.gz
echo "Running test for fuzzy_light/00000057.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000057.log.gz
echo "Running test for fuzzy_light/00000058.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000058.log.gz
echo "Running test for fuzzy_light/00000059.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000059.log.gz
echo "Running test for fuzzy_light/00000060.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000060.log.gz
echo "Running test for fuzzy_light/00000061.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000061.log.gz
echo "Running test for fuzzy_light/00000062.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000062.log.gz
echo "Running test for fuzzy_light/00000063.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000063.log.gz
echo "Running test for fuzzy_light/00000064.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000064.log.gz
echo "Running test for fuzzy_light/00000065.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000065.log.gz
echo "Running test for fuzzy_light/00000066.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000066.log.gz
echo "Running test for fuzzy_light/00000067.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000067.log.gz
echo "Running test for fuzzy_light/00000068.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000068.log.gz
echo "Running test for fuzzy_light/00000069.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000069.log.gz
echo "Running test for fuzzy_light/00000070.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000070.log.gz
echo "Running test for fuzzy_light/00000071.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000071.log.gz
echo "Running test for fuzzy_light/00000072.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000072.log.gz
echo "Running test for fuzzy_light/00000073.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000073.log.gz
echo "Running test for fuzzy_light/00000074.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000074.log.gz
echo "Running test for fuzzy_light/00000075.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000075.log.gz
echo "Running test for fuzzy_light/00000076.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000076.log.gz
echo "Running test for fuzzy_light/00000077.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000077.log.gz
echo "Running test for fuzzy_light/00000078.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000078.log.gz
echo "Running test for fuzzy_light/00000079.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000079.log.gz
echo "Running test for fuzzy_light/00000080.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000080.log.gz
echo "Running test for fuzzy_light/00000081.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000081.log.gz
echo "Running test for fuzzy_light/00000082.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000082.log.gz
echo "Running test for fuzzy_light/00000083.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000083.log.gz
echo "Running test for fuzzy_light/00000084.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000084.log.gz
echo "Running test for fuzzy_light/00000085.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000085.log.gz
echo "Running test for fuzzy_light/00000086.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000086.log.gz
echo "Running test for fuzzy_light/00000087.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000087.log.gz
echo "Running test for fuzzy_light/00000088.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000088.log.gz
echo "Running test for fuzzy_light/00000089.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000089.log.gz
echo "Running test for fuzzy_light/00000090.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000090.log.gz
echo "Running test for fuzzy_light/00000091.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000091.log.gz
echo "Running test for fuzzy_light/00000092.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000092.log.gz
echo "Running test for fuzzy_light/00000093.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000093.log.gz
echo "Running test for fuzzy_light/00000094.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000094.log.gz
echo "Running test for fuzzy_light/00000095.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000095.log.gz
echo "Running test for fuzzy_light/00000096.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000096.log.gz
echo "Running test for fuzzy_light/00000097.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000097.log.gz
echo "Running test for fuzzy_light/00000098.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000098.log.gz
echo "Running test for fuzzy_light/00000099.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000099.log.gz
echo "Running test for fuzzy_light/00000100.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000100.log.gz
echo "Running test for fuzzy_light/00000101.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000101.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000101.log.gz
echo "Running test for fuzzy_light/00000102.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000102.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000102.log.gz
echo "Running test for fuzzy_light/00000103.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000103.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000103.log.gz
echo "Running test for fuzzy_light/00000104.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000104.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000104.log.gz
echo "Running test for fuzzy_light/00000105.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000105.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000105.log.gz
echo "Running test for fuzzy_light/00000106.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000106.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000106.log.gz
echo "Running test for fuzzy_light/00000107.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000107.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000107.log.gz
echo "Running test for fuzzy_light/00000108.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000108.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000108.log.gz
echo "Running test for fuzzy_light/00000109.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000109.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000109.log.gz
echo "Running test for fuzzy_light/00000110.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000110.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000110.log.gz
echo "Running test for fuzzy_light/00000111.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000111.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000111.log.gz
echo "Running test for fuzzy_light/00000112.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000112.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000112.log.gz
echo "Running test for fuzzy_light/00000113.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000113.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000113.log.gz
echo "Running test for fuzzy_light/00000114.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000114.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000114.log.gz
echo "Running test for fuzzy_light/00000115.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000115.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000115.log.gz
echo "Running test for fuzzy_light/00000116.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000116.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000116.log.gz
echo "Running test for fuzzy_light/00000117.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000117.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000117.log.gz
echo "Running test for fuzzy_light/00000118.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000118.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000118.log.gz
echo "Running test for fuzzy_light/00000119.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000119.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000119.log.gz
echo "Running test for fuzzy_light/00000120.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000120.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000120.log.gz
echo "Running test for fuzzy_light/00000121.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000121.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000121.log.gz
echo "Running test for fuzzy_light/00000122.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000122.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000122.log.gz
echo "Running test for fuzzy_light/00000123.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000123.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000123.log.gz
echo "Running test for fuzzy_light/00000124.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000124.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000124.log.gz
echo "Running test for fuzzy_light/00000125.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000125.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000125.log.gz
echo "Running test for fuzzy_light/00000126.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000126.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000126.log.gz
echo "Running test for fuzzy_light/00000127.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000127.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000127.log.gz
echo "Running test for fuzzy_light/00000128.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000128.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000128.log.gz
echo "Running test for fuzzy_light/00000129.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000129.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000129.log.gz
echo "Running test for fuzzy_light/00000130.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000130.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000130.log.gz
echo "Running test for fuzzy_light/00000131.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000131.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000131.log.gz
echo "Running test for fuzzy_light/00000132.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000132.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000132.log.gz
echo "Running test for fuzzy_light/00000133.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000133.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000133.log.gz
echo "Running test for fuzzy_light/00000134.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000134.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000134.log.gz
echo "Running test for fuzzy_light/00000135.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000135.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000135.log.gz
echo "Running test for fuzzy_light/00000136.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000136.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000136.log.gz
echo "Running test for fuzzy_light/00000137.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000137.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000137.log.gz
echo "Running test for fuzzy_light/00000138.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000138.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000138.log.gz
echo "Running test for fuzzy_light/00000139.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000139.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000139.log.gz
echo "Running test for fuzzy_light/00000140.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000140.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000140.log.gz
echo "Running test for fuzzy_light/00000141.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000141.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000141.log.gz
echo "Running test for fuzzy_light/00000142.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000142.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000142.log.gz
echo "Running test for fuzzy_light/00000143.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000143.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000143.log.gz
echo "Running test for fuzzy_light/00000144.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000144.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000144.log.gz
echo "Running test for fuzzy_light/00000145.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000145.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000145.log.gz
echo "Running test for fuzzy_light/00000146.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000146.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000146.log.gz
echo "Running test for fuzzy_light/00000147.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000147.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000147.log.gz
echo "Running test for fuzzy_light/00000148.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000148.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000148.log.gz
echo "Running test for fuzzy_light/00000149.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000149.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000149.log.gz
echo "Running test for fuzzy_light/00000150.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000150.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000150.log.gz
echo "Running test for fuzzy_light/00000151.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000151.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000151.log.gz
echo "Running test for fuzzy_light/00000152.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000152.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000152.log.gz
echo "Running test for fuzzy_light/00000153.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000153.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000153.log.gz
echo "Running test for fuzzy_light/00000154.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000154.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000154.log.gz
echo "Running test for fuzzy_light/00000155.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000155.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000155.log.gz
echo "Running test for fuzzy_light/00000156.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000156.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000156.log.gz
echo "Running test for fuzzy_light/00000157.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000157.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000157.log.gz
echo "Running test for fuzzy_light/00000158.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000158.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000158.log.gz
echo "Running test for fuzzy_light/00000159.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000159.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000159.log.gz
echo "Running test for fuzzy_light/00000160.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000160.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000160.log.gz
echo "Running test for fuzzy_light/00000161.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000161.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000161.log.gz
echo "Running test for fuzzy_light/00000162.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000162.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000162.log.gz
echo "Running test for fuzzy_light/00000163.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000163.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000163.log.gz
echo "Running test for fuzzy_light/00000164.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000164.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000164.log.gz
echo "Running test for fuzzy_light/00000165.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000165.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000165.log.gz
echo "Running test for fuzzy_light/00000166.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000166.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000166.log.gz
echo "Running test for fuzzy_light/00000167.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000167.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000167.log.gz
echo "Running test for fuzzy_light/00000168.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000168.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000168.log.gz
echo "Running test for fuzzy_light/00000169.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000169.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000169.log.gz
echo "Running test for fuzzy_light/00000170.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000170.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000170.log.gz
echo "Running test for fuzzy_light/00000171.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000171.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000171.log.gz
echo "Running test for fuzzy_light/00000172.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000172.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000172.log.gz
echo "Running test for fuzzy_light/00000173.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000173.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000173.log.gz
echo "Running test for fuzzy_light/00000174.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000174.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000174.log.gz
echo "Running test for fuzzy_light/00000175.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000175.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000175.log.gz
echo "Running test for fuzzy_light/00000176.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000176.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000176.log.gz
echo "Running test for fuzzy_light/00000177.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000177.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000177.log.gz
echo "Running test for fuzzy_light/00000178.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000178.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000178.log.gz
echo "Running test for fuzzy_light/00000179.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000179.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000179.log.gz
echo "Running test for fuzzy_light/00000180.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000180.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000180.log.gz
echo "Running test for fuzzy_light/00000181.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000181.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000181.log.gz
echo "Running test for fuzzy_light/00000182.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000182.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000182.log.gz
echo "Running test for fuzzy_light/00000183.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000183.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000183.log.gz
echo "Running test for fuzzy_light/00000184.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000184.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000184.log.gz
echo "Running test for fuzzy_light/00000185.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000185.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000185.log.gz
echo "Running test for fuzzy_light/00000186.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000186.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000186.log.gz
echo "Running test for fuzzy_light/00000187.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000187.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000187.log.gz
echo "Running test for fuzzy_light/00000188.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000188.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000188.log.gz
echo "Running test for fuzzy_light/00000189.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000189.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000189.log.gz
echo "Running test for fuzzy_light/00000190.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000190.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000190.log.gz
echo "Running test for fuzzy_light/00000191.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000191.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000191.log.gz
echo "Running test for fuzzy_light/00000192.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000192.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000192.log.gz
echo "Running test for fuzzy_light/00000193.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000193.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000193.log.gz
echo "Running test for fuzzy_light/00000194.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000194.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000194.log.gz
echo "Running test for fuzzy_light/00000195.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000195.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000195.log.gz
echo "Running test for fuzzy_light/00000196.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000196.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000196.log.gz
echo "Running test for fuzzy_light/00000197.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000197.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000197.log.gz
echo "Running test for fuzzy_light/00000198.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000198.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000198.log.gz
echo "Running test for fuzzy_light/00000199.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000199.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000199.log.gz
echo "Running test for fuzzy_light/00000200.bin..."
go test -run="TestTracesGoInterpreter/fuzzy_light/00000200.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/fuzzy_light/00000200.log.gz

# Processing preimages directory
echo "Running test for preimages/00000001.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000001.log.gz
echo "Running test for preimages/00000002.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000002.log.gz
echo "Running test for preimages/00000003.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000003.log.gz
echo "Running test for preimages/00000004.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000004.log.gz
echo "Running test for preimages/00000005.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000005.log.gz
echo "Running test for preimages/00000006.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000006.log.gz
echo "Running test for preimages/00000007.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000007.log.gz
echo "Running test for preimages/00000008.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000008.log.gz
echo "Running test for preimages/00000009.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000009.log.gz
echo "Running test for preimages/00000010.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000010.log.gz
echo "Running test for preimages/00000011.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000011.log.gz
echo "Running test for preimages/00000012.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000012.log.gz
echo "Running test for preimages/00000013.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000013.log.gz
echo "Running test for preimages/00000014.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000014.log.gz
echo "Running test for preimages/00000015.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000015.log.gz
echo "Running test for preimages/00000016.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000016.log.gz
echo "Running test for preimages/00000017.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000017.log.gz
echo "Running test for preimages/00000018.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000018.log.gz
echo "Running test for preimages/00000019.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000019.log.gz
echo "Running test for preimages/00000020.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000020.log.gz
echo "Running test for preimages/00000021.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000021.log.gz
echo "Running test for preimages/00000022.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000022.log.gz
echo "Running test for preimages/00000023.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000023.log.gz
echo "Running test for preimages/00000024.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000024.log.gz
echo "Running test for preimages/00000025.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000025.log.gz
echo "Running test for preimages/00000026.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000026.log.gz
echo "Running test for preimages/00000027.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000027.log.gz
echo "Running test for preimages/00000028.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000028.log.gz
echo "Running test for preimages/00000029.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000029.log.gz
echo "Running test for preimages/00000030.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000030.log.gz
echo "Running test for preimages/00000031.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000031.log.gz
echo "Running test for preimages/00000032.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000032.log.gz
echo "Running test for preimages/00000033.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000033.log.gz
echo "Running test for preimages/00000034.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000034.log.gz
echo "Running test for preimages/00000035.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000035.log.gz
echo "Running test for preimages/00000036.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000036.log.gz
echo "Running test for preimages/00000037.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000037.log.gz
echo "Running test for preimages/00000038.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000038.log.gz
echo "Running test for preimages/00000039.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000039.log.gz
echo "Running test for preimages/00000040.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000040.log.gz
echo "Running test for preimages/00000041.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000041.log.gz
echo "Running test for preimages/00000042.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000042.log.gz
echo "Running test for preimages/00000043.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000043.log.gz
echo "Running test for preimages/00000044.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000044.log.gz
echo "Running test for preimages/00000045.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000045.log.gz
echo "Running test for preimages/00000046.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000046.log.gz
echo "Running test for preimages/00000047.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000047.log.gz
echo "Running test for preimages/00000048.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000048.log.gz
echo "Running test for preimages/00000049.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000049.log.gz
echo "Running test for preimages/00000050.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000050.log.gz
echo "Running test for preimages/00000051.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000051.log.gz
echo "Running test for preimages/00000052.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000052.log.gz
echo "Running test for preimages/00000053.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000053.log.gz
echo "Running test for preimages/00000054.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000054.log.gz
echo "Running test for preimages/00000055.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000055.log.gz
echo "Running test for preimages/00000056.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000056.log.gz
echo "Running test for preimages/00000057.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000057.log.gz
echo "Running test for preimages/00000058.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000058.log.gz
echo "Running test for preimages/00000059.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000059.log.gz
echo "Running test for preimages/00000060.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000060.log.gz
echo "Running test for preimages/00000061.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000061.log.gz
echo "Running test for preimages/00000062.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000062.log.gz
echo "Running test for preimages/00000063.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000063.log.gz
echo "Running test for preimages/00000064.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000064.log.gz
echo "Running test for preimages/00000065.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000065.log.gz
echo "Running test for preimages/00000066.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000066.log.gz
echo "Running test for preimages/00000067.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000067.log.gz
echo "Running test for preimages/00000068.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000068.log.gz
echo "Running test for preimages/00000069.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000069.log.gz
echo "Running test for preimages/00000070.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000070.log.gz
echo "Running test for preimages/00000071.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000071.log.gz
echo "Running test for preimages/00000072.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000072.log.gz
echo "Running test for preimages/00000073.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000073.log.gz
echo "Running test for preimages/00000074.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000074.log.gz
echo "Running test for preimages/00000075.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000075.log.gz
echo "Running test for preimages/00000076.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000076.log.gz
echo "Running test for preimages/00000077.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000077.log.gz
echo "Running test for preimages/00000078.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000078.log.gz
echo "Running test for preimages/00000079.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000079.log.gz
echo "Running test for preimages/00000080.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000080.log.gz
echo "Running test for preimages/00000081.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000081.log.gz
echo "Running test for preimages/00000082.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000082.log.gz
echo "Running test for preimages/00000083.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000083.log.gz
echo "Running test for preimages/00000084.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000084.log.gz
echo "Running test for preimages/00000085.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000085.log.gz
echo "Running test for preimages/00000086.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000086.log.gz
echo "Running test for preimages/00000087.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000087.log.gz
echo "Running test for preimages/00000088.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000088.log.gz
echo "Running test for preimages/00000089.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000089.log.gz
echo "Running test for preimages/00000090.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000090.log.gz
echo "Running test for preimages/00000091.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000091.log.gz
echo "Running test for preimages/00000092.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000092.log.gz
echo "Running test for preimages/00000093.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000093.log.gz
echo "Running test for preimages/00000094.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000094.log.gz
echo "Running test for preimages/00000095.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000095.log.gz
echo "Running test for preimages/00000096.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000096.log.gz
echo "Running test for preimages/00000097.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000097.log.gz
echo "Running test for preimages/00000098.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000098.log.gz
echo "Running test for preimages/00000099.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000099.log.gz
echo "Running test for preimages/00000100.bin..."
go test -run="TestTracesGoInterpreter/preimages/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages/00000100.log.gz

# Processing preimages_light directory
echo "Running test for preimages_light/00000001.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000001.log.gz
echo "Running test for preimages_light/00000002.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000002.log.gz
echo "Running test for preimages_light/00000003.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000003.log.gz
echo "Running test for preimages_light/00000004.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000004.log.gz
echo "Running test for preimages_light/00000005.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000005.log.gz
echo "Running test for preimages_light/00000006.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000006.log.gz
echo "Running test for preimages_light/00000007.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000007.log.gz
echo "Running test for preimages_light/00000008.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000008.log.gz
echo "Running test for preimages_light/00000009.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000009.log.gz
echo "Running test for preimages_light/00000010.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000010.log.gz
echo "Running test for preimages_light/00000011.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000011.log.gz
echo "Running test for preimages_light/00000012.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000012.log.gz
echo "Running test for preimages_light/00000013.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000013.log.gz
echo "Running test for preimages_light/00000014.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000014.log.gz
echo "Running test for preimages_light/00000015.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000015.log.gz
echo "Running test for preimages_light/00000016.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000016.log.gz
echo "Running test for preimages_light/00000017.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000017.log.gz
echo "Running test for preimages_light/00000018.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000018.log.gz
echo "Running test for preimages_light/00000019.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000019.log.gz
echo "Running test for preimages_light/00000020.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000020.log.gz
echo "Running test for preimages_light/00000021.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000021.log.gz
echo "Running test for preimages_light/00000022.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000022.log.gz
echo "Running test for preimages_light/00000023.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000023.log.gz
echo "Running test for preimages_light/00000024.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000024.log.gz
echo "Running test for preimages_light/00000025.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000025.log.gz
echo "Running test for preimages_light/00000026.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000026.log.gz
echo "Running test for preimages_light/00000027.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000027.log.gz
echo "Running test for preimages_light/00000028.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000028.log.gz
echo "Running test for preimages_light/00000029.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000029.log.gz
echo "Running test for preimages_light/00000030.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000030.log.gz
echo "Running test for preimages_light/00000031.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000031.log.gz
echo "Running test for preimages_light/00000032.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000032.log.gz
echo "Running test for preimages_light/00000033.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000033.log.gz
echo "Running test for preimages_light/00000034.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000034.log.gz
echo "Running test for preimages_light/00000035.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000035.log.gz
echo "Running test for preimages_light/00000036.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000036.log.gz
echo "Running test for preimages_light/00000037.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000037.log.gz
echo "Running test for preimages_light/00000038.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000038.log.gz
echo "Running test for preimages_light/00000039.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000039.log.gz
echo "Running test for preimages_light/00000040.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000040.log.gz
echo "Running test for preimages_light/00000041.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000041.log.gz
echo "Running test for preimages_light/00000042.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000042.log.gz
echo "Running test for preimages_light/00000043.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000043.log.gz
echo "Running test for preimages_light/00000044.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000044.log.gz
echo "Running test for preimages_light/00000045.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000045.log.gz
echo "Running test for preimages_light/00000046.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000046.log.gz
echo "Running test for preimages_light/00000047.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000047.log.gz
echo "Running test for preimages_light/00000048.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000048.log.gz
echo "Running test for preimages_light/00000049.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000049.log.gz
echo "Running test for preimages_light/00000050.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000050.log.gz
echo "Running test for preimages_light/00000051.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000051.log.gz
echo "Running test for preimages_light/00000052.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000052.log.gz
echo "Running test for preimages_light/00000053.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000053.log.gz
echo "Running test for preimages_light/00000054.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000054.log.gz
echo "Running test for preimages_light/00000055.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000055.log.gz
echo "Running test for preimages_light/00000056.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000056.log.gz
echo "Running test for preimages_light/00000057.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000057.log.gz
echo "Running test for preimages_light/00000058.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000058.log.gz
echo "Running test for preimages_light/00000059.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000059.log.gz
echo "Running test for preimages_light/00000060.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000060.log.gz
echo "Running test for preimages_light/00000061.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000061.log.gz
echo "Running test for preimages_light/00000062.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000062.log.gz
echo "Running test for preimages_light/00000063.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000063.log.gz
echo "Running test for preimages_light/00000064.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000064.log.gz
echo "Running test for preimages_light/00000065.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000065.log.gz
echo "Running test for preimages_light/00000066.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000066.log.gz
echo "Running test for preimages_light/00000067.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000067.log.gz
echo "Running test for preimages_light/00000068.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000068.log.gz
echo "Running test for preimages_light/00000069.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000069.log.gz
echo "Running test for preimages_light/00000070.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000070.log.gz
echo "Running test for preimages_light/00000071.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000071.log.gz
echo "Running test for preimages_light/00000072.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000072.log.gz
echo "Running test for preimages_light/00000073.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000073.log.gz
echo "Running test for preimages_light/00000074.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000074.log.gz
echo "Running test for preimages_light/00000075.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000075.log.gz
echo "Running test for preimages_light/00000076.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000076.log.gz
echo "Running test for preimages_light/00000077.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000077.log.gz
echo "Running test for preimages_light/00000078.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000078.log.gz
echo "Running test for preimages_light/00000079.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000079.log.gz
echo "Running test for preimages_light/00000080.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000080.log.gz
echo "Running test for preimages_light/00000081.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000081.log.gz
echo "Running test for preimages_light/00000082.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000082.log.gz
echo "Running test for preimages_light/00000083.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000083.log.gz
echo "Running test for preimages_light/00000084.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000084.log.gz
echo "Running test for preimages_light/00000085.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000085.log.gz
echo "Running test for preimages_light/00000086.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000086.log.gz
echo "Running test for preimages_light/00000087.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000087.log.gz
echo "Running test for preimages_light/00000088.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000088.log.gz
echo "Running test for preimages_light/00000089.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000089.log.gz
echo "Running test for preimages_light/00000090.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000090.log.gz
echo "Running test for preimages_light/00000091.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000091.log.gz
echo "Running test for preimages_light/00000092.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000092.log.gz
echo "Running test for preimages_light/00000093.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000093.log.gz
echo "Running test for preimages_light/00000094.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000094.log.gz
echo "Running test for preimages_light/00000095.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000095.log.gz
echo "Running test for preimages_light/00000096.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000096.log.gz
echo "Running test for preimages_light/00000097.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000097.log.gz
echo "Running test for preimages_light/00000098.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000098.log.gz
echo "Running test for preimages_light/00000099.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000099.log.gz
echo "Running test for preimages_light/00000100.bin..."
go test -run="TestTracesGoInterpreter/preimages_light/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/preimages_light/00000100.log.gz

# Processing storage directory
echo "Running test for storage/00000001.bin..."
go test -run="TestTracesGoInterpreter/storage/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000001.log.gz
echo "Running test for storage/00000002.bin..."
go test -run="TestTracesGoInterpreter/storage/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000002.log.gz
echo "Running test for storage/00000003.bin..."
go test -run="TestTracesGoInterpreter/storage/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000003.log.gz
echo "Running test for storage/00000004.bin..."
go test -run="TestTracesGoInterpreter/storage/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000004.log.gz
echo "Running test for storage/00000005.bin..."
go test -run="TestTracesGoInterpreter/storage/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000005.log.gz
echo "Running test for storage/00000006.bin..."
go test -run="TestTracesGoInterpreter/storage/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000006.log.gz
echo "Running test for storage/00000007.bin..."
go test -run="TestTracesGoInterpreter/storage/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000007.log.gz
echo "Running test for storage/00000008.bin..."
go test -run="TestTracesGoInterpreter/storage/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000008.log.gz
echo "Running test for storage/00000009.bin..."
go test -run="TestTracesGoInterpreter/storage/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000009.log.gz
echo "Running test for storage/00000010.bin..."
go test -run="TestTracesGoInterpreter/storage/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000010.log.gz
echo "Running test for storage/00000011.bin..."
go test -run="TestTracesGoInterpreter/storage/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000011.log.gz
echo "Running test for storage/00000012.bin..."
go test -run="TestTracesGoInterpreter/storage/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000012.log.gz
echo "Running test for storage/00000013.bin..."
go test -run="TestTracesGoInterpreter/storage/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000013.log.gz
echo "Running test for storage/00000014.bin..."
go test -run="TestTracesGoInterpreter/storage/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000014.log.gz
echo "Running test for storage/00000015.bin..."
go test -run="TestTracesGoInterpreter/storage/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000015.log.gz
echo "Running test for storage/00000016.bin..."
go test -run="TestTracesGoInterpreter/storage/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000016.log.gz
echo "Running test for storage/00000017.bin..."
go test -run="TestTracesGoInterpreter/storage/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000017.log.gz
echo "Running test for storage/00000018.bin..."
go test -run="TestTracesGoInterpreter/storage/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000018.log.gz
echo "Running test for storage/00000019.bin..."
go test -run="TestTracesGoInterpreter/storage/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000019.log.gz
echo "Running test for storage/00000020.bin..."
go test -run="TestTracesGoInterpreter/storage/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000020.log.gz
echo "Running test for storage/00000021.bin..."
go test -run="TestTracesGoInterpreter/storage/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000021.log.gz
echo "Running test for storage/00000022.bin..."
go test -run="TestTracesGoInterpreter/storage/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000022.log.gz
echo "Running test for storage/00000023.bin..."
go test -run="TestTracesGoInterpreter/storage/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000023.log.gz
echo "Running test for storage/00000024.bin..."
go test -run="TestTracesGoInterpreter/storage/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000024.log.gz
echo "Running test for storage/00000025.bin..."
go test -run="TestTracesGoInterpreter/storage/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000025.log.gz
echo "Running test for storage/00000026.bin..."
go test -run="TestTracesGoInterpreter/storage/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000026.log.gz
echo "Running test for storage/00000027.bin..."
go test -run="TestTracesGoInterpreter/storage/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000027.log.gz
echo "Running test for storage/00000028.bin..."
go test -run="TestTracesGoInterpreter/storage/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000028.log.gz
echo "Running test for storage/00000029.bin..."
go test -run="TestTracesGoInterpreter/storage/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000029.log.gz
echo "Running test for storage/00000030.bin..."
go test -run="TestTracesGoInterpreter/storage/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000030.log.gz
echo "Running test for storage/00000031.bin..."
go test -run="TestTracesGoInterpreter/storage/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000031.log.gz
echo "Running test for storage/00000032.bin..."
go test -run="TestTracesGoInterpreter/storage/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000032.log.gz
echo "Running test for storage/00000033.bin..."
go test -run="TestTracesGoInterpreter/storage/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000033.log.gz
echo "Running test for storage/00000034.bin..."
go test -run="TestTracesGoInterpreter/storage/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000034.log.gz
echo "Running test for storage/00000035.bin..."
go test -run="TestTracesGoInterpreter/storage/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000035.log.gz
echo "Running test for storage/00000036.bin..."
go test -run="TestTracesGoInterpreter/storage/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000036.log.gz
echo "Running test for storage/00000037.bin..."
go test -run="TestTracesGoInterpreter/storage/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000037.log.gz
echo "Running test for storage/00000038.bin..."
go test -run="TestTracesGoInterpreter/storage/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000038.log.gz
echo "Running test for storage/00000039.bin..."
go test -run="TestTracesGoInterpreter/storage/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000039.log.gz
echo "Running test for storage/00000040.bin..."
go test -run="TestTracesGoInterpreter/storage/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000040.log.gz
echo "Running test for storage/00000041.bin..."
go test -run="TestTracesGoInterpreter/storage/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000041.log.gz
echo "Running test for storage/00000042.bin..."
go test -run="TestTracesGoInterpreter/storage/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000042.log.gz
echo "Running test for storage/00000043.bin..."
go test -run="TestTracesGoInterpreter/storage/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000043.log.gz
echo "Running test for storage/00000044.bin..."
go test -run="TestTracesGoInterpreter/storage/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000044.log.gz
echo "Running test for storage/00000045.bin..."
go test -run="TestTracesGoInterpreter/storage/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000045.log.gz
echo "Running test for storage/00000046.bin..."
go test -run="TestTracesGoInterpreter/storage/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000046.log.gz
echo "Running test for storage/00000047.bin..."
go test -run="TestTracesGoInterpreter/storage/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000047.log.gz
echo "Running test for storage/00000048.bin..."
go test -run="TestTracesGoInterpreter/storage/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000048.log.gz
echo "Running test for storage/00000049.bin..."
go test -run="TestTracesGoInterpreter/storage/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000049.log.gz
echo "Running test for storage/00000050.bin..."
go test -run="TestTracesGoInterpreter/storage/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000050.log.gz
echo "Running test for storage/00000051.bin..."
go test -run="TestTracesGoInterpreter/storage/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000051.log.gz
echo "Running test for storage/00000052.bin..."
go test -run="TestTracesGoInterpreter/storage/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000052.log.gz
echo "Running test for storage/00000053.bin..."
go test -run="TestTracesGoInterpreter/storage/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000053.log.gz
echo "Running test for storage/00000054.bin..."
go test -run="TestTracesGoInterpreter/storage/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000054.log.gz
echo "Running test for storage/00000055.bin..."
go test -run="TestTracesGoInterpreter/storage/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000055.log.gz
echo "Running test for storage/00000056.bin..."
go test -run="TestTracesGoInterpreter/storage/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000056.log.gz
echo "Running test for storage/00000057.bin..."
go test -run="TestTracesGoInterpreter/storage/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000057.log.gz
echo "Running test for storage/00000058.bin..."
go test -run="TestTracesGoInterpreter/storage/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000058.log.gz
echo "Running test for storage/00000059.bin..."
go test -run="TestTracesGoInterpreter/storage/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000059.log.gz
echo "Running test for storage/00000060.bin..."
go test -run="TestTracesGoInterpreter/storage/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000060.log.gz
echo "Running test for storage/00000061.bin..."
go test -run="TestTracesGoInterpreter/storage/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000061.log.gz
echo "Running test for storage/00000062.bin..."
go test -run="TestTracesGoInterpreter/storage/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000062.log.gz
echo "Running test for storage/00000063.bin..."
go test -run="TestTracesGoInterpreter/storage/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000063.log.gz
echo "Running test for storage/00000064.bin..."
go test -run="TestTracesGoInterpreter/storage/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000064.log.gz
echo "Running test for storage/00000065.bin..."
go test -run="TestTracesGoInterpreter/storage/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000065.log.gz
echo "Running test for storage/00000066.bin..."
go test -run="TestTracesGoInterpreter/storage/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000066.log.gz
echo "Running test for storage/00000067.bin..."
go test -run="TestTracesGoInterpreter/storage/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000067.log.gz
echo "Running test for storage/00000068.bin..."
go test -run="TestTracesGoInterpreter/storage/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000068.log.gz
echo "Running test for storage/00000069.bin..."
go test -run="TestTracesGoInterpreter/storage/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000069.log.gz
echo "Running test for storage/00000070.bin..."
go test -run="TestTracesGoInterpreter/storage/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000070.log.gz
echo "Running test for storage/00000071.bin..."
go test -run="TestTracesGoInterpreter/storage/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000071.log.gz
echo "Running test for storage/00000072.bin..."
go test -run="TestTracesGoInterpreter/storage/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000072.log.gz
echo "Running test for storage/00000073.bin..."
go test -run="TestTracesGoInterpreter/storage/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000073.log.gz
echo "Running test for storage/00000074.bin..."
go test -run="TestTracesGoInterpreter/storage/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000074.log.gz
echo "Running test for storage/00000075.bin..."
go test -run="TestTracesGoInterpreter/storage/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000075.log.gz
echo "Running test for storage/00000076.bin..."
go test -run="TestTracesGoInterpreter/storage/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000076.log.gz
echo "Running test for storage/00000077.bin..."
go test -run="TestTracesGoInterpreter/storage/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000077.log.gz
echo "Running test for storage/00000078.bin..."
go test -run="TestTracesGoInterpreter/storage/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000078.log.gz
echo "Running test for storage/00000079.bin..."
go test -run="TestTracesGoInterpreter/storage/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000079.log.gz
echo "Running test for storage/00000080.bin..."
go test -run="TestTracesGoInterpreter/storage/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000080.log.gz
echo "Running test for storage/00000081.bin..."
go test -run="TestTracesGoInterpreter/storage/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000081.log.gz
echo "Running test for storage/00000082.bin..."
go test -run="TestTracesGoInterpreter/storage/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000082.log.gz
echo "Running test for storage/00000083.bin..."
go test -run="TestTracesGoInterpreter/storage/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000083.log.gz
echo "Running test for storage/00000084.bin..."
go test -run="TestTracesGoInterpreter/storage/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000084.log.gz
echo "Running test for storage/00000085.bin..."
go test -run="TestTracesGoInterpreter/storage/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000085.log.gz
echo "Running test for storage/00000086.bin..."
go test -run="TestTracesGoInterpreter/storage/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000086.log.gz
echo "Running test for storage/00000087.bin..."
go test -run="TestTracesGoInterpreter/storage/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000087.log.gz
echo "Running test for storage/00000088.bin..."
go test -run="TestTracesGoInterpreter/storage/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000088.log.gz
echo "Running test for storage/00000089.bin..."
go test -run="TestTracesGoInterpreter/storage/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000089.log.gz
echo "Running test for storage/00000090.bin..."
go test -run="TestTracesGoInterpreter/storage/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000090.log.gz
echo "Running test for storage/00000091.bin..."
go test -run="TestTracesGoInterpreter/storage/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000091.log.gz
echo "Running test for storage/00000092.bin..."
go test -run="TestTracesGoInterpreter/storage/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000092.log.gz
echo "Running test for storage/00000093.bin..."
go test -run="TestTracesGoInterpreter/storage/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000093.log.gz
echo "Running test for storage/00000094.bin..."
go test -run="TestTracesGoInterpreter/storage/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000094.log.gz
echo "Running test for storage/00000095.bin..."
go test -run="TestTracesGoInterpreter/storage/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000095.log.gz
echo "Running test for storage/00000096.bin..."
go test -run="TestTracesGoInterpreter/storage/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000096.log.gz
echo "Running test for storage/00000097.bin..."
go test -run="TestTracesGoInterpreter/storage/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000097.log.gz
echo "Running test for storage/00000098.bin..."
go test -run="TestTracesGoInterpreter/storage/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000098.log.gz
echo "Running test for storage/00000099.bin..."
go test -run="TestTracesGoInterpreter/storage/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000099.log.gz
echo "Running test for storage/00000100.bin..."
go test -run="TestTracesGoInterpreter/storage/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage/00000100.log.gz

# Processing storage_light directory
echo "Running test for storage_light/00000001.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000001.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000001.log.gz
echo "Running test for storage_light/00000002.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000002.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000002.log.gz
echo "Running test for storage_light/00000003.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000003.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000003.log.gz
echo "Running test for storage_light/00000004.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000004.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000004.log.gz
echo "Running test for storage_light/00000005.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000005.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000005.log.gz
echo "Running test for storage_light/00000006.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000006.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000006.log.gz
echo "Running test for storage_light/00000007.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000007.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000007.log.gz
echo "Running test for storage_light/00000008.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000008.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000008.log.gz
echo "Running test for storage_light/00000009.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000009.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000009.log.gz
echo "Running test for storage_light/00000010.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000010.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000010.log.gz
echo "Running test for storage_light/00000011.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000011.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000011.log.gz
echo "Running test for storage_light/00000012.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000012.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000012.log.gz
echo "Running test for storage_light/00000013.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000013.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000013.log.gz
echo "Running test for storage_light/00000014.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000014.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000014.log.gz
echo "Running test for storage_light/00000015.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000015.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000015.log.gz
echo "Running test for storage_light/00000016.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000016.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000016.log.gz
echo "Running test for storage_light/00000017.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000017.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000017.log.gz
echo "Running test for storage_light/00000018.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000018.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000018.log.gz
echo "Running test for storage_light/00000019.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000019.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000019.log.gz
echo "Running test for storage_light/00000020.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000020.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000020.log.gz
echo "Running test for storage_light/00000021.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000021.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000021.log.gz
echo "Running test for storage_light/00000022.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000022.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000022.log.gz
echo "Running test for storage_light/00000023.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000023.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000023.log.gz
echo "Running test for storage_light/00000024.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000024.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000024.log.gz
echo "Running test for storage_light/00000025.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000025.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000025.log.gz
echo "Running test for storage_light/00000026.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000026.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000026.log.gz
echo "Running test for storage_light/00000027.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000027.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000027.log.gz
echo "Running test for storage_light/00000028.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000028.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000028.log.gz
echo "Running test for storage_light/00000029.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000029.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000029.log.gz
echo "Running test for storage_light/00000030.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000030.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000030.log.gz
echo "Running test for storage_light/00000031.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000031.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000031.log.gz
echo "Running test for storage_light/00000032.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000032.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000032.log.gz
echo "Running test for storage_light/00000033.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000033.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000033.log.gz
echo "Running test for storage_light/00000034.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000034.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000034.log.gz
echo "Running test for storage_light/00000035.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000035.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000035.log.gz
echo "Running test for storage_light/00000036.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000036.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000036.log.gz
echo "Running test for storage_light/00000037.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000037.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000037.log.gz
echo "Running test for storage_light/00000038.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000038.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000038.log.gz
echo "Running test for storage_light/00000039.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000039.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000039.log.gz
echo "Running test for storage_light/00000040.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000040.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000040.log.gz
echo "Running test for storage_light/00000041.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000041.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000041.log.gz
echo "Running test for storage_light/00000042.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000042.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000042.log.gz
echo "Running test for storage_light/00000043.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000043.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000043.log.gz
echo "Running test for storage_light/00000044.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000044.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000044.log.gz
echo "Running test for storage_light/00000045.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000045.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000045.log.gz
echo "Running test for storage_light/00000046.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000046.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000046.log.gz
echo "Running test for storage_light/00000047.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000047.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000047.log.gz
echo "Running test for storage_light/00000048.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000048.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000048.log.gz
echo "Running test for storage_light/00000049.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000049.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000049.log.gz
echo "Running test for storage_light/00000050.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000050.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000050.log.gz
echo "Running test for storage_light/00000051.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000051.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000051.log.gz
echo "Running test for storage_light/00000052.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000052.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000052.log.gz
echo "Running test for storage_light/00000053.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000053.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000053.log.gz
echo "Running test for storage_light/00000054.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000054.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000054.log.gz
echo "Running test for storage_light/00000055.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000055.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000055.log.gz
echo "Running test for storage_light/00000056.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000056.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000056.log.gz
echo "Running test for storage_light/00000057.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000057.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000057.log.gz
echo "Running test for storage_light/00000058.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000058.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000058.log.gz
echo "Running test for storage_light/00000059.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000059.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000059.log.gz
echo "Running test for storage_light/00000060.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000060.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000060.log.gz
echo "Running test for storage_light/00000061.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000061.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000061.log.gz
echo "Running test for storage_light/00000062.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000062.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000062.log.gz
echo "Running test for storage_light/00000063.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000063.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000063.log.gz
echo "Running test for storage_light/00000064.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000064.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000064.log.gz
echo "Running test for storage_light/00000065.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000065.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000065.log.gz
echo "Running test for storage_light/00000066.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000066.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000066.log.gz
echo "Running test for storage_light/00000067.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000067.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000067.log.gz
echo "Running test for storage_light/00000068.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000068.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000068.log.gz
echo "Running test for storage_light/00000069.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000069.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000069.log.gz
echo "Running test for storage_light/00000070.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000070.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000070.log.gz
echo "Running test for storage_light/00000071.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000071.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000071.log.gz
echo "Running test for storage_light/00000072.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000072.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000072.log.gz
echo "Running test for storage_light/00000073.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000073.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000073.log.gz
echo "Running test for storage_light/00000074.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000074.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000074.log.gz
echo "Running test for storage_light/00000075.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000075.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000075.log.gz
echo "Running test for storage_light/00000076.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000076.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000076.log.gz
echo "Running test for storage_light/00000077.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000077.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000077.log.gz
echo "Running test for storage_light/00000078.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000078.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000078.log.gz
echo "Running test for storage_light/00000079.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000079.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000079.log.gz
echo "Running test for storage_light/00000080.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000080.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000080.log.gz
echo "Running test for storage_light/00000081.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000081.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000081.log.gz
echo "Running test for storage_light/00000082.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000082.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000082.log.gz
echo "Running test for storage_light/00000083.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000083.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000083.log.gz
echo "Running test for storage_light/00000084.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000084.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000084.log.gz
echo "Running test for storage_light/00000085.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000085.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000085.log.gz
echo "Running test for storage_light/00000086.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000086.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000086.log.gz
echo "Running test for storage_light/00000087.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000087.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000087.log.gz
echo "Running test for storage_light/00000088.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000088.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000088.log.gz
echo "Running test for storage_light/00000089.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000089.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000089.log.gz
echo "Running test for storage_light/00000090.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000090.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000090.log.gz
echo "Running test for storage_light/00000091.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000091.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000091.log.gz
echo "Running test for storage_light/00000092.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000092.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000092.log.gz
echo "Running test for storage_light/00000093.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000093.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000093.log.gz
echo "Running test for storage_light/00000094.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000094.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000094.log.gz
echo "Running test for storage_light/00000095.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000095.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000095.log.gz
echo "Running test for storage_light/00000096.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000096.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000096.log.gz
echo "Running test for storage_light/00000097.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000097.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000097.log.gz
echo "Running test for storage_light/00000098.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000098.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000098.log.gz
echo "Running test for storage_light/00000099.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000099.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000099.log.gz
echo "Running test for storage_light/00000100.bin..."
go test -run="TestTracesGoInterpreter/storage_light/00000100.bin$" 2>&1 | gzip > /Users/sourabhniyogi/Desktop/jamtestnet/traces/0.7.1/storage_light/00000100.log.gz

echo "All tests completed!"
