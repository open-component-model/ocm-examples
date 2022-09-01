# Demo Component 

This component provides a simple OCM component describing a podinfo
Kubernetes deployment.

The following operations are supported by the Makefile:

- Create a CTF Archive

  ```
  $ make ctf
  ```

- Sign the CTF Archive

  ```
  $ PRIVKEY=<path to private key file> [SIGNATURE=<signature name>] make sign 
  ```

- Publish the Component Version

  ```
  $ [PRIVKEY=<path to private key file> [SIGNATURE=<signature name>]] make push 
  ```

  Add the optionaly parts if you want to sign it.

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
