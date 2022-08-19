#!/usr/bin/env python

import kubernetes

def main():
    print(kubernetes.client.ApiextensionsV1beta1Api.create_custom_resource_definition)

if __name__ == '__main__':
    main()
