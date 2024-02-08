FROM scratch
WORKDIR /app
COPY teldrive /
EXPOSE 8080
ENTRYPOINT ["/app/teldrive"]