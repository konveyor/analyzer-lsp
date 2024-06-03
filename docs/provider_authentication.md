# Provider Authentication

In some instances, you may be running a provider on an external server, and you will want to make sure that only analyzer's and users that should have access to the GRPC endpoints can make the calls to the provider.s

## Using TLS

To enable Authentication you must be using TLS. To enable TLS, you will need to tell the provider the certificate and key, and in the provider settings you will need to tell it the certificate to use. You can use self signed signed certs to complete this. 

**Note: that the CN must be the hostname that you will use in the address to connect to the provider.**

### Example Cert Generation LocalHost

```sh
 openssl req  -newkey rsa:2048  -x509  -nodes  -keyout keytmp.pem  -new  -out cert.pem  -subj /CN=localhost  -reqexts SAN  -extensions SAN  -config <(cat /System/Library/OpenSSL/openssl.cnf  <(printf '[SAN]\nsubjectAltName=DNS:localhost'))  -sha256  -days 3650
```

### Example Provider Start Up

```sh
java-provider --certFile <path-to-cert> --keyFile <path-to-key>
```

Each provider could implement this differently, but for all the in-tree providers these flags are standard.

### Example Provider Config

```yaml
    ...
    {
        "name": "java",
        "address":  "localhost:14651",
        certPath: <path-to-cert>,
        initConfig: {...}
    }
```

## Setting up the Provider

To set up an external provider that uses the default server implementation library you need to provide a secert key, that you use to sign a JWT you will use in the provider config.

Generating JWT's can be done via [CLI](https://gist.github.com/indrayam/dd47bf6eef849a57c07016c0036f5207) or through [jwt.io](https://jwt.io).

### Example Provider Start Up

```sh
java-provider --certFile <path-to-cert> --keyFile <path-to-key> --secretKey <your super secret key>
```

Each provider could implement this differently, but for all the in-tree providers these flags are standard.

### Example Provider Config

```yaml
    ...
    {
        "name": "java",
        "address":  "localhost:14651",
        certPath: <path-to-cert>,
        jwtToken: <jwt-token>
        initConfig: {...}
    }
```