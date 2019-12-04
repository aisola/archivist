Archivist
=========

Archivist is a simple file-uploading service for applications. Essentailly, it provides a single endpoint which allows
users to upload a single file to a backend file host. It was initially designed to only support Backblaze B2, but
eventually, I assume it will be updated to allow more backends as necessary.

### API

```shell script
curl -X POST -H 'Content-Type: image/jpeg' -H 'Archivist-File-Name: whatever-you-want-the-backend-file-name-to-be.jpg' --data-binary "@your-file.jpg" http://archivist-url/
```

Response:

```json
{
  "id": "4_z523eef997c94922f67b4031c_f109bec7495c4f827_d20191204_m004917_c000_v0001064_t0000",
  "media_type": "image/jpeg",
  "name": "whatever-you-want-the-backend-file-name-to-be.jpg",
  "sha1": "52a6943aaf1ea6827a8199aeb5bfad84b3b3f861",
  "size": 4759595
}
```
