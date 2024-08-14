FROM golang:bookworm 

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update && apt install build-essential gcc pkg-config libwebkit2gtk-4.0-dev libjson-glib-dev -y

WORKDIR /
RUN useradd -ms /bin/bash onedriver
RUN mkdir /mount && chown onedriver:onedriver -R /mount

USER onedriver
WORKDIR /build
COPY --chown=onedriver:onedriver . .