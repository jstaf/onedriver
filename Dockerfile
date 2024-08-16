FROM golang:bookworm 

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update && apt -y install build-essential fuse 

RUN groupadd onedriver
RUN useradd -ms /bin/bash onedriver
VOLUME [ "/home/onedriver/.cache/onedriver" ]
RUN mkdir /mount && chown onedriver:onedriver -R /mount

USER onedriver
WORKDIR /build
COPY --chown=onedriver:onedriver . .
RUN make onedriver-headless

ENTRYPOINT [ "/build/onedriver-headless", "--no-browser", "/mount/" ]