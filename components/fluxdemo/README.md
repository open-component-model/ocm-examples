# Demo Component 

This component provides a simple OCM component describing a podinfo
Kubernetes deployment.

The component version can be built, signed, published, transported
and verified just by using make targets.
The make targets use the *ocm* client tool to do the job. They can be
used as prototype to on builds and to demonstrate the usage of the 
command line tool.

The following operations are supported by the Makefile:

- Create a CTF Archive

  ```
  $ make ctf
  ```

- Create RSA keypair for signing, it will be stored in folder `local`

  ```
  $ make keys 
  ```

- Sign the CTF Archive

  ```
  $ KEY=<path to private key file> [SIGNATURE=<signature name>] make sign 
  ```

- Publish the Component Version

  ```
  $ [KEY=<path to private key file> [SIGNATURE=<signature name>]] make push 
  ```

  Add the optional parts if you want to sign it.

- Verify signature in (remote) repository

  ```
  $ [KEY=<path to public/private key file> [SIGNATURE=<signature name>] [OCMREPO=<target repository>] make verify 

- Transport from OCM delivery repository to a given target repository

  ```
  $ TARGETREPO=<ocm target repository> [OCMREPO=<ocm source repository>] [OCMREPO=<target repository>] make transport 
  ```

- Show delivered content

  ```
  $ make describe 
  ```

- Show Component Descriptor

  ```
  $ make descriptor 
  ```


- Cleanup everything

  ```
  $ make clean 
  ```

 
If another ocm repository should be used, it can be specified by setting the variable `OCMREPO`.
