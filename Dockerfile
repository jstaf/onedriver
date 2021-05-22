FROM golang:1-buster
RUN apt-get update
RUN apt-get -y --no-install-recommends install \
    gcc pkg-config 
RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN make onedriver-headless
