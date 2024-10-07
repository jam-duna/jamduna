# Use a Unix-based image, such as Debian
FROM debian:latest
WORKDIR /jamweb
COPY web /jamweb
EXPOSE 80
RUN chmod +x /jamweb/jamweb
CMD ["./web"]



