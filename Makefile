GO = go

all: build

build:
	${GO} build -v -o gcs-importer ./importer
	${GO} build -v -o gcs-exporter ./exporter
	${GO} build -v -o gcs-storage  ./storage

clean:
	rm -f gcs-importer gcs-exporter gcs-storage
