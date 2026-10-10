FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/teldrive /usr/bin/teldrive
EXPOSE 8080
ENTRYPOINT ["/usr/bin/teldrive"]
CMD ["serve"]
