#!/bin/bash

while getopts p: flag
do
	case "${flag}" in
		p) path=${OPTARG};;
	esac
done

cd $path

go mod graph