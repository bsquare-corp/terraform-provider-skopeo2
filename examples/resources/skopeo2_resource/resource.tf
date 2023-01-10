resource "skopeo2_copy" "example" {
  source_image = "<source image>"
  destination_image = "<dest image>"
  preserve_digests = true
}
