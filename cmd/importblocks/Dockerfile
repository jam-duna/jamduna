# Use a Unix-based image, such as Debian
FROM debian:latest
WORKDIR /jam
ENV CARGO_MANIFEST_DIR=/jam
ENV JAM_PATH=/jam
COPY importblocks /jam
COPY bandersnatch/data/zcash-srs-2-11-uncompressed.bin /jam/data/zcash-srs-2-11-uncompressed.bin
EXPOSE 9000/udp
RUN chmod +x /jam/importblocks
CMD ["/jam/importblocks", "--port=9000"]
