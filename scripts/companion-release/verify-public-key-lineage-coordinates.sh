#!/usr/bin/env bash

# @AX:NOTE [AUTO]: Append a phase only after immutable evidence pins exist; every phase trusts its direct predecessor.
readonly A0_REPOSITORY='Insajin/autopus-adk' A0_TAG='v0.50.69' A0_VERSION='0.50.69'
readonly A1_REPOSITORY='Insajin/autopus-adk' A1_TAG='v0.50.70' A1_VERSION='0.50.70'
readonly A2_REPOSITORY='Insajin/autopus-adk' A2_TAG='v0.50.71' A2_VERSION='0.50.71'
readonly A3_TAG='v0.50.72' A3_VERSION='0.50.72'
readonly A3_REPOSITORY='Insajin/autopus-adk' A4_TAG='v0.50.73' A4_VERSION='0.50.73'
readonly A4_REPOSITORY='Insajin/autopus-adk' A5_TAG='v0.50.74' A5_VERSION='0.50.74'
readonly A5_REPOSITORY='Insajin/autopus-adk' A6_TAG='v0.50.77' A6_VERSION='0.50.77'
readonly A6_REPOSITORY='Insajin/autopus-adk' A7_TAG='v0.50.78' A7_VERSION='0.50.78'
readonly A7_REPOSITORY='Insajin/autopus-adk' A8_TAG='v0.50.79' A8_VERSION='0.50.79'
readonly A8_REPOSITORY='Insajin/autopus-adk' A9_TAG='v0.50.80' A9_VERSION='0.50.80'
readonly A9_REPOSITORY='Insajin/autopus-adk' A10_TAG='v0.50.81' A10_VERSION='0.50.81'
readonly A10_REPOSITORY='Insajin/autopus-adk' A11_TAG='v0.50.82' A11_VERSION='0.50.82'
readonly A11_REPOSITORY='Insajin/autopus-adk' A12_TAG='v0.50.83' A12_VERSION='0.50.83'
readonly A12_REPOSITORY='Insajin/autopus-adk' A13_TAG='v0.50.84' A13_VERSION='0.50.84'
readonly A13_REPOSITORY='Insajin/autopus-adk' A14_TAG='v0.50.85' A14_VERSION='0.50.85'
readonly A14_REPOSITORY='Insajin/autopus-adk' A15_TAG='v0.50.86' A15_VERSION='0.50.86'
readonly A15_REPOSITORY='Insajin/autopus-adk' A16_TAG='v0.50.87' A16_VERSION='0.50.87'
readonly A16_REPOSITORY='Insajin/autopus-adk' A17_TAG='v0.50.88' A17_VERSION='0.50.88'
readonly A17_REPOSITORY='Insajin/autopus-adk' A18_TAG='v0.50.89' A18_VERSION='0.50.89'
readonly A18_REPOSITORY='Insajin/autopus-adk' A19_TAG='v0.50.90' A19_VERSION='0.50.90'
readonly A19_REPOSITORY='Insajin/autopus-adk' A20_TAG='v0.50.91' A20_VERSION='0.50.91'
readonly A20_REPOSITORY='Insajin/autopus-adk' A21_TAG='v0.50.92' A21_VERSION='0.50.92'
readonly A21_REPOSITORY='Insajin/autopus-adk' A22_TAG='v0.50.109' A22_VERSION='0.50.109'
readonly A22_REPOSITORY='Insajin/autopus-adk' A23_TAG='v0.50.111' A23_VERSION='0.50.111'
readonly A23_REPOSITORY='Insajin/autopus-adk' A24_TAG='v0.50.113' A24_VERSION='0.50.113'
# The A23 through A30 predecessor pins follow A22's in
# verify-public-key-lineage-pins.sh, which the lineage verifier sources first,
# and the A31 through A40 pins are in verify-public-key-lineage-shipped-pins.sh,
# which it sources next. They moved there unchanged, block by block, as this
# file reached the 300-line source limit. Pins from A41 on are declared below.
readonly A24_REPOSITORY='Insajin/autopus-adk' A25_TAG='v0.50.114' A25_VERSION='0.50.114'
readonly A25_REPOSITORY='Insajin/autopus-adk' A26_TAG='v0.50.115' A26_VERSION='0.50.115'
readonly A26_REPOSITORY='Insajin/autopus-adk' A27_TAG='v0.50.116' A27_VERSION='0.50.116'
readonly A27_REPOSITORY='Insajin/autopus-adk' A28_TAG='v0.50.117' A28_VERSION='0.50.117'
readonly A28_REPOSITORY='Insajin/autopus-adk' A29_TAG='v0.50.118' A29_VERSION='0.50.118'
readonly A29_REPOSITORY='autopus-ai/autopus-adk' A30_TAG='v0.50.119' A30_VERSION='0.50.119'
readonly A30_REPOSITORY='autopus-ai/autopus-adk' A31_TAG='v0.50.120' A31_VERSION='0.50.120'
readonly A31_REPOSITORY='autopus-ai/autopus-adk' A32_TAG='v0.50.121' A32_VERSION='0.50.121'
readonly A32_REPOSITORY='autopus-ai/autopus-adk' A33_TAG='v0.50.122' A33_VERSION='0.50.122'
readonly A33_REPOSITORY='autopus-ai/autopus-adk' A34_TAG='v0.50.123' A34_VERSION='0.50.123'
readonly A34_REPOSITORY='autopus-ai/autopus-adk' A35_TAG='v0.50.125' A35_VERSION='0.50.125'
readonly A35_REPOSITORY='autopus-ai/autopus-adk' A36_TAG='v0.50.127' A36_VERSION='0.50.127'
readonly A36_REPOSITORY='autopus-ai/autopus-adk' A37_TAG='v0.50.128' A37_VERSION='0.50.128'
readonly A37_REPOSITORY='autopus-ai/autopus-adk' A38_TAG='v0.50.131' A38_VERSION='0.50.131'
readonly A38_REPOSITORY='autopus-ai/autopus-adk' A39_TAG='v0.50.132' A39_VERSION='0.50.132'
readonly A39_REPOSITORY='autopus-ai/autopus-adk' A40_TAG='v0.50.133' A40_VERSION='0.50.133'
readonly A40_REPOSITORY='autopus-ai/autopus-adk' A41_TAG='v0.51.0' A41_VERSION='0.51.0'
readonly A41_REPOSITORY='autopus-ai/autopus-adk' A42_TAG='v0.51.1' A42_VERSION='0.51.1'
# A41 shipped the same fifteen-asset set as A40 under the same owner, all
# measured from the immutable release 439021686. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A41_RELEASE_ID='439021686'
readonly A41_COMMIT_SHA='ce109c5ae958f86b624bc40e91aaf7f6b1bf0f6d'
readonly A41_TREE_SHA='c857ba9ff2ba14743ba605e9d822c36f4280d048'
readonly A41_TAG_OBJECT_SHA='30a77fb9a291fc33d76db2f6b89d7e1e86f92a27'
readonly A41_CHECKSUMS_SHA256='489b3846eb6f98558b4e19b38c32381e87fa8e7cbbb41b6e512280c42bd5e078'
readonly A41_AMD64_ARCHIVE_SHA256='445789e557180fb3a15210aed76482318c9526644597548141883ff5c7255fb9'
readonly A41_ARM64_ARCHIVE_SHA256='c355c6c841172df59aa76784996648ad86ddc4f8e949ec37fbd315b6c25629ad'
readonly A41_LINUX_AMD64_ARCHIVE_SHA256='7db5fa4cd36e3aac584d1a53b87c2de55a81a817874a6cfb07ce642e66e75413'
readonly A41_LINUX_ARM64_ARCHIVE_SHA256='88ca3ae74aab3213a61f1895e543a119d43a7046aa624412e9c3370e62623dc9'
readonly A41_AMD64_MANIFEST_SHA256='8cb887d87bcd1d66be134bb960b9aab89e30f3517f16788226ffe3648ad421f9'
readonly A41_ARM64_MANIFEST_SHA256='b70a778b4e0d9904f87dcb5506c05c96950027b5b39d901681e5c0ba398c00cc'
readonly A42_REPOSITORY='autopus-ai/autopus-adk' A43_TAG='v0.51.2' A43_VERSION='0.51.2'
# A42 shipped the same fifteen-asset set as A41 under the same owner, all
# measured from the immutable release 439292420. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A42_RELEASE_ID='439292420'
readonly A42_COMMIT_SHA='48c4a16538ef4ab829c50b7852f8217477300f53'
readonly A42_TREE_SHA='6e7b9c8b44980ef935deb96fa41e52c87e169196'
readonly A42_TAG_OBJECT_SHA='6df9e3dc4ed4deac09d6527ae253bd055c3c9538'
readonly A42_CHECKSUMS_SHA256='f5d09a7ba471f7ade69cbce3060d2fc72d9ae7ca621ca08a904f808628cafd07'
readonly A42_AMD64_ARCHIVE_SHA256='5a13a7cb3c3820be047d7f4e9716c7884b1b58896e02ceadc1ff9929367b47f3'
readonly A42_ARM64_ARCHIVE_SHA256='ca9a128cc17f023a98cd754e5b4c2841268bf766668589e168b55aa4af77a525'
readonly A42_LINUX_AMD64_ARCHIVE_SHA256='f8236176b6dbf8ea54c178b0061c3125f3836c46a19129167ec455a2e387e8ee'
readonly A42_LINUX_ARM64_ARCHIVE_SHA256='393a453a9727cf5422f9237d3f22bc6bc19aa934788d19946e556dad95d6dc7b'
readonly A42_AMD64_MANIFEST_SHA256='15c467efce430bea6059b18bc6503713e4c693ab76c23418bc19822dcdc5c9c5'
readonly A42_ARM64_MANIFEST_SHA256='cedaa9534c923e68d522ab1da0cbd3fdcacee97bf92b13abc2bd63a84c56495d'
readonly A43_REPOSITORY='autopus-ai/autopus-adk' A44_TAG='v0.51.3' A44_VERSION='0.51.3'
# A43 shipped the same fifteen-asset set as A42 under the same owner, all
# measured from the immutable release 439814195. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A43_RELEASE_ID='439814195'
readonly A43_COMMIT_SHA='276d4bd16a9aec143688ab19cfd6affd01f28b00'
readonly A43_TREE_SHA='70918efee2ca1a129dc575804359e20bc31a9826'
readonly A43_TAG_OBJECT_SHA='83b79d29a8fc82d8a7e38cd1fd122e69bb19f641'
readonly A43_CHECKSUMS_SHA256='6beb50075b7c6cf19873d6499c2ccb50dd2bfcf6d3750582dff79026d26821be'
readonly A43_AMD64_ARCHIVE_SHA256='b5cea2f82cc3970c40c83a94fd641c416cbf31915e01fd1622969339f7cf95d0'
readonly A43_ARM64_ARCHIVE_SHA256='4696f73a1a57202353a7ed3a157b92294f4e736fcc8401e229c3a5775104c86f'
readonly A43_LINUX_AMD64_ARCHIVE_SHA256='66a9e1973ec211f861354d2701b214a3afb20e4a91803c84d99ecd02dfd641bc'
readonly A43_LINUX_ARM64_ARCHIVE_SHA256='c157b14822fb6601bcd093b4aefba92865bd81302efb791273c82a08e0df3076'
readonly A43_AMD64_MANIFEST_SHA256='5e998973604386c94fe608ccaceb908b8d8e9e816a5c2e949ea2f8b58f91cd70'
readonly A43_ARM64_MANIFEST_SHA256='fe99e5eca171c2cdc8f5b28eaaa6a8d882298c4d8c9d71a79d871675498ee1f8'
readonly A44_REPOSITORY='autopus-ai/autopus-adk' A45_TAG='v0.51.4' A45_VERSION='0.51.4'
# A44 shipped the same fifteen-asset set as A43 under the same owner, all
# measured from the immutable release 440878772. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A44_RELEASE_ID='440878772'
readonly A44_COMMIT_SHA='4a0fb20ca37ea54efe611484d697e1e79ba0b8f1'
readonly A44_TREE_SHA='56804d3f8118d08fa536a0171b472717c9a7cf33'
readonly A44_TAG_OBJECT_SHA='e274a155aef5790be8ba769657bd5a19bacf9c10'
readonly A44_CHECKSUMS_SHA256='fed21d1c14597bc37b2f6231076859362fc881f18fbae3d7137666c61e09c7ee'
readonly A44_AMD64_ARCHIVE_SHA256='018ccdc92179faadcce32f24a89277021cfde1ae548320160d248015445cb0b9'
readonly A44_ARM64_ARCHIVE_SHA256='5c3377ebd802e6806bb94010288d82ead5910444e225cdbc9b10a56cdd2b6dfd'
readonly A44_LINUX_AMD64_ARCHIVE_SHA256='6512d19fc2ff316ebecabe749d3c887f57a2be313037df7445e97a74408f6e76'
readonly A44_LINUX_ARM64_ARCHIVE_SHA256='3364759308c582118e696342dd6a3852b17a84ac41ed787106249723f6b8ac73'
readonly A44_AMD64_MANIFEST_SHA256='2db97fd2fbd5e74c992916a3353833b3397b1de187b9c419d57431cfd2c21462'
readonly A44_ARM64_MANIFEST_SHA256='8e82aba2309f6bc3260515c47c9c8360cc1fed57d4f26b53f9f126b3105ef51d'
readonly A45_REPOSITORY='autopus-ai/autopus-adk' A46_TAG='v0.51.5' A46_VERSION='0.51.5'
# A45 shipped the same fifteen-asset set as A44 under the same owner, all
# measured from the immutable release 441368606. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A45_RELEASE_ID='441368606'
readonly A45_COMMIT_SHA='4312105591a74f79ed3f62a7023423d49a2944f0'
readonly A45_TREE_SHA='77458576f6e16a10312d5222e08a04e35000efc6'
readonly A45_TAG_OBJECT_SHA='9130626401937001ccb2366faf62f0f4184534d9'
readonly A45_CHECKSUMS_SHA256='610ef4032e3b7ce28e4845825c6b55091d5ded8b40c9e2c4ea635c6cd23e1140'
readonly A45_AMD64_ARCHIVE_SHA256='08c5de85142d36b35f79f09f0d35433c83b2cf5a1b252e3bdc1d40f041fdeb44'
readonly A45_ARM64_ARCHIVE_SHA256='97d26ea4da3df491e79d0e338eb0ae8d5ef7c529fc09172ebc148dc13464ab92'
readonly A45_LINUX_AMD64_ARCHIVE_SHA256='c8a71813f4f1c7b6822c2353003bdd60a99adcd07fceeb91237844b18e09698d'
readonly A45_LINUX_ARM64_ARCHIVE_SHA256='ab90b16f104a9766cde740246f15183c6fc880d2e6cd187b65bac4b67134207d'
readonly A45_AMD64_MANIFEST_SHA256='380c4b20060924832e223e8841dbc2bb4db57d7fcd7e3dfd2fece9cee613efd5'
readonly A45_ARM64_MANIFEST_SHA256='18499b51e5bd4413b8ee50223ea4343de036859bc81b4063344a02b9ca6cab95'
readonly A46_REPOSITORY='autopus-ai/autopus-adk' A47_TAG='v0.51.6' A47_VERSION='0.51.6'
# A46 shipped the same fifteen-asset set as A45 under the same owner, all
# measured from the immutable release 441700303. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A46_RELEASE_ID='441700303'
readonly A46_COMMIT_SHA='8ab8b182c425ab8a68112293e8ab272ff3765dcb'
readonly A46_TREE_SHA='cc3d28854ecf838a391092162e38d196b955a9c4'
readonly A46_TAG_OBJECT_SHA='12941460a548c4bf09014a1c2486937121c14d7a'
readonly A46_CHECKSUMS_SHA256='a1e3ec35684f6790f9f0fb177b560431762a35215116de1b860834d0250ecb8b'
readonly A46_AMD64_ARCHIVE_SHA256='d201eb1e71b80a15f4a0572e822849f9b93bc45b56dbe6172114dd9b0f6a7c88'
readonly A46_ARM64_ARCHIVE_SHA256='d2d70469de12a18c10057880c2c054ce6221632758633ae44d7393ee02b4f56b'
readonly A46_LINUX_AMD64_ARCHIVE_SHA256='71d3ba3a6eb952ae02ff1d1283b8c264992757de8a6879274f29d2f54274c247'
readonly A46_LINUX_ARM64_ARCHIVE_SHA256='5fc4b125a67484c1d9852a9d01102ee940fc98d7c2135ef4bf239a4896d8a311'
readonly A46_AMD64_MANIFEST_SHA256='e5bf9a1bb867c0f408482d08efa820c25c58d343c881a00dcd3dbd8fa2b5c704'
readonly A46_ARM64_MANIFEST_SHA256='9571791d6e27398b98332aaec8a870e7bca4bc509e8565939f0afc75f6d7159f'
readonly A47_REPOSITORY='autopus-ai/autopus-adk' A48_TAG='v0.51.7' A48_VERSION='0.51.7'
# A47 shipped the same fifteen-asset set as A46 under the same owner, all
# measured from the immutable release 443543192. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A47_RELEASE_ID='443543192'
readonly A47_COMMIT_SHA='ea183465fa5955c683cca89f9b5e2bd5e2c8ed09'
readonly A47_TREE_SHA='57343b01666226b9d339ef49edd691ba1905b236'
readonly A47_TAG_OBJECT_SHA='dd14ac35c7e1c66c0f2da2d64450c6a507b23a32'
readonly A47_CHECKSUMS_SHA256='9ab4f3b3f77397e0117a22c491216e18c8b8eac7274b23762731650a387104d6'
readonly A47_AMD64_ARCHIVE_SHA256='e0039113ecbd5018c13288598b2667dde5f3b7e7476ba5ace2580b4a71967999'
readonly A47_ARM64_ARCHIVE_SHA256='4912c458f39742579b1d63f6402143355e896bd018695bd09acc236b0d8dd102'
readonly A47_LINUX_AMD64_ARCHIVE_SHA256='9f386f3cc8186934b9cce5e4916cd543e024f830ecf33ebbf2cc1ecab82fa442'
readonly A47_LINUX_ARM64_ARCHIVE_SHA256='31ad18bce25e375e162b1535455d0c65ad43891098ca552b992f43019abbb283'
readonly A47_AMD64_MANIFEST_SHA256='8106005d25c547bc26c81d826d811e48be1304efe88bf5f3cf97c557f58cfc32'
readonly A47_ARM64_MANIFEST_SHA256='42c839f4193b0a95294b5914709feee0971b1967018777fd538008f0525daf11'
readonly A0_EVIDENCE_SOURCE='immutable A0 GitHub release'

require_environment GITHUB_REF_NAME
COMPANION_VERSION="${GITHUB_REF_NAME#v}"
prior_release_id='' prior_tree='' prior_linux_amd64_archive='' prior_linux_arm64_archive=''
if [[ "$GITHUB_REF_NAME" == 'v0.50.69' && "$COMPANION_VERSION" == '0.50.69' ]]; then
  release_phase='A0'
  printf 'companion release lineage: %s bootstrap accepted for %s@%s\n' "$release_phase" "$A0_REPOSITORY" "$A0_TAG"
  exit 0
elif [[ "$GITHUB_REF_NAME" == "$A1_TAG" && "$COMPANION_VERSION" == "$A1_VERSION" ]]; then
  release_phase='A1' prior_phase='A0' prior_repository="$A0_REPOSITORY" prior_evidence_source="$A0_EVIDENCE_SOURCE"
  prior_tag="$A0_TAG" prior_version="$A0_VERSION" prior_commit="$A0_COMMIT_SHA"
  prior_tag_object='' prior_checksums="$A0_CHECKSUMS_SHA256" prior_amd64_archive='' prior_arm64_archive=''
  prior_amd64_manifest="$A0_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A0_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A2_TAG" && "$COMPANION_VERSION" == "$A2_VERSION" ]]; then
  release_phase='A2' prior_phase='A1' prior_repository="$A1_REPOSITORY" prior_evidence_source='immutable A1 GitHub release'
  prior_tag="$A1_TAG" prior_version="$A1_VERSION" prior_commit="$A1_COMMIT_SHA"
  prior_tag_object="$A1_TAG_OBJECT_SHA" prior_checksums="$A1_CHECKSUMS_SHA256" prior_amd64_archive="$A1_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A1_ARM64_ARCHIVE_SHA256"
  prior_amd64_manifest="$A1_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A1_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A3_TAG" && "$COMPANION_VERSION" == "$A3_VERSION" ]]; then
  release_phase='A3' prior_phase='A2' prior_repository="$A2_REPOSITORY" prior_evidence_source='immutable A2 GitHub release'
  prior_tag="$A2_TAG" prior_version="$A2_VERSION" prior_commit="$A2_COMMIT_SHA"
  prior_tag_object="$A2_TAG_OBJECT_SHA" prior_checksums="$A2_CHECKSUMS_SHA256" prior_amd64_archive="$A2_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A2_ARM64_ARCHIVE_SHA256"
  prior_amd64_manifest="$A2_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A2_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A4_TAG" && "$COMPANION_VERSION" == "$A4_VERSION" ]]; then
  release_phase='A4' prior_phase='A3' prior_repository="$A3_REPOSITORY" prior_evidence_source='immutable A3 GitHub release' prior_tag="$A3_TAG" prior_version="$A3_VERSION" prior_commit="$A3_COMMIT_SHA"
  prior_tag_object="$A3_TAG_OBJECT_SHA" prior_checksums="$A3_CHECKSUMS_SHA256" prior_amd64_archive="$A3_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A3_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A3_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A3_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A5_TAG" && "$COMPANION_VERSION" == "$A5_VERSION" ]]; then
  release_phase='A5' prior_phase='A4' prior_repository="$A4_REPOSITORY" prior_evidence_source='immutable A4 GitHub release' prior_tag="$A4_TAG" prior_version="$A4_VERSION" prior_commit="$A4_COMMIT_SHA"
  prior_tag_object="$A4_TAG_OBJECT_SHA" prior_checksums="$A4_CHECKSUMS_SHA256" prior_amd64_archive="$A4_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A4_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A4_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A4_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A6_TAG" && "$COMPANION_VERSION" == "$A6_VERSION" ]]; then
  release_phase='A6' prior_phase='A5' prior_repository="$A5_REPOSITORY" prior_evidence_source='immutable A5 GitHub release' prior_tag="$A5_TAG" prior_version="$A5_VERSION" prior_commit="$A5_COMMIT_SHA"
  prior_tag_object="$A5_TAG_OBJECT_SHA" prior_checksums="$A5_CHECKSUMS_SHA256" prior_amd64_archive="$A5_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A5_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A5_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A5_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A7_TAG" && "$COMPANION_VERSION" == "$A7_VERSION" ]]; then
  release_phase='A7' prior_phase='A6' prior_repository="$A6_REPOSITORY" prior_evidence_source='immutable A6 GitHub release' prior_tag="$A6_TAG" prior_version="$A6_VERSION" prior_commit="$A6_COMMIT_SHA"
  prior_tag_object="$A6_TAG_OBJECT_SHA" prior_checksums="$A6_CHECKSUMS_SHA256" prior_amd64_archive="$A6_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A6_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A6_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A6_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A8_TAG" && "$COMPANION_VERSION" == "$A8_VERSION" ]]; then
  release_phase='A8' prior_phase='A7' prior_repository="$A7_REPOSITORY" prior_evidence_source='immutable A7 GitHub release' prior_tag="$A7_TAG" prior_version="$A7_VERSION" prior_commit="$A7_COMMIT_SHA" prior_tree="$A7_TREE_SHA"
  prior_tag_object="$A7_TAG_OBJECT_SHA" prior_checksums="$A7_CHECKSUMS_SHA256" prior_amd64_archive="$A7_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A7_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A7_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A7_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A9_TAG" && "$COMPANION_VERSION" == "$A9_VERSION" ]]; then
  release_phase='A9' prior_phase='A8' prior_repository="$A8_REPOSITORY" prior_evidence_source='immutable A8 GitHub release' prior_tag="$A8_TAG" prior_version="$A8_VERSION" prior_commit="$A8_COMMIT_SHA" prior_tree="$A8_TREE_SHA"
  prior_tag_object="$A8_TAG_OBJECT_SHA" prior_checksums="$A8_CHECKSUMS_SHA256" prior_amd64_archive="$A8_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A8_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A8_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A8_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A10_TAG" && "$COMPANION_VERSION" == "$A10_VERSION" ]]; then
  release_phase='A10' prior_phase='A9' prior_repository="$A9_REPOSITORY" prior_evidence_source='immutable A9 GitHub release' prior_tag="$A9_TAG" prior_version="$A9_VERSION" prior_commit="$A9_COMMIT_SHA" prior_tree="$A9_TREE_SHA"
  prior_tag_object="$A9_TAG_OBJECT_SHA" prior_checksums="$A9_CHECKSUMS_SHA256" prior_amd64_archive="$A9_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A9_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A9_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A9_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A11_TAG" && "$COMPANION_VERSION" == "$A11_VERSION" ]]; then
  release_phase='A11' prior_phase='A10' prior_repository="$A10_REPOSITORY" prior_evidence_source='immutable A10 GitHub release' prior_tag="$A10_TAG" prior_version="$A10_VERSION" prior_commit="$A10_COMMIT_SHA" prior_tree="$A10_TREE_SHA"
  prior_tag_object="$A10_TAG_OBJECT_SHA" prior_checksums="$A10_CHECKSUMS_SHA256" prior_amd64_archive="$A10_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A10_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A10_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A10_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A12_TAG" && "$COMPANION_VERSION" == "$A12_VERSION" ]]; then
  release_phase='A12' prior_phase='A11' prior_repository="$A11_REPOSITORY" prior_evidence_source='immutable A11 GitHub release' prior_tag="$A11_TAG" prior_version="$A11_VERSION" prior_commit="$A11_COMMIT_SHA" prior_tree="$A11_TREE_SHA"
  prior_tag_object="$A11_TAG_OBJECT_SHA" prior_checksums="$A11_CHECKSUMS_SHA256" prior_amd64_archive="$A11_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A11_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A11_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A11_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A13_TAG" && "$COMPANION_VERSION" == "$A13_VERSION" ]]; then
  release_phase='A13' prior_phase='A12' prior_repository="$A12_REPOSITORY" prior_evidence_source='immutable A12 GitHub release' prior_tag="$A12_TAG" prior_version="$A12_VERSION" prior_commit="$A12_COMMIT_SHA" prior_tree="$A12_TREE_SHA"
  prior_tag_object="$A12_TAG_OBJECT_SHA" prior_checksums="$A12_CHECKSUMS_SHA256" prior_amd64_archive="$A12_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A12_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A12_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A12_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A14_TAG" && "$COMPANION_VERSION" == "$A14_VERSION" ]]; then
  release_phase='A14' prior_phase='A13' prior_repository="$A13_REPOSITORY" prior_evidence_source='immutable A13 GitHub release' prior_tag="$A13_TAG" prior_version="$A13_VERSION" prior_commit="$A13_COMMIT_SHA" prior_tree="$A13_TREE_SHA"
  prior_tag_object="$A13_TAG_OBJECT_SHA" prior_checksums="$A13_CHECKSUMS_SHA256" prior_amd64_archive="$A13_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A13_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A13_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A13_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A15_TAG" && "$COMPANION_VERSION" == "$A15_VERSION" ]]; then
  release_phase='A15' prior_phase='A14' prior_repository="$A14_REPOSITORY" prior_evidence_source='immutable A14 GitHub release' prior_tag="$A14_TAG" prior_version="$A14_VERSION" prior_commit="$A14_COMMIT_SHA" prior_tree="$A14_TREE_SHA"
  prior_tag_object="$A14_TAG_OBJECT_SHA" prior_checksums="$A14_CHECKSUMS_SHA256" prior_amd64_archive="$A14_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A14_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A14_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A14_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A14_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A14_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A16_TAG" && "$COMPANION_VERSION" == "$A16_VERSION" ]]; then
  release_phase='A16' prior_phase='A15' prior_repository="$A15_REPOSITORY" prior_evidence_source='immutable A15 GitHub release' prior_tag="$A15_TAG" prior_version="$A15_VERSION" prior_commit="$A15_COMMIT_SHA" prior_tree="$A15_TREE_SHA"
  prior_tag_object="$A15_TAG_OBJECT_SHA" prior_checksums="$A15_CHECKSUMS_SHA256" prior_amd64_archive="$A15_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A15_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A15_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A15_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A15_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A15_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A17_TAG" && "$COMPANION_VERSION" == "$A17_VERSION" ]]; then
  release_phase='A17' prior_phase='A16' prior_repository="$A16_REPOSITORY" prior_evidence_source='immutable A16 GitHub release' prior_tag="$A16_TAG" prior_version="$A16_VERSION" prior_commit="$A16_COMMIT_SHA" prior_tree="$A16_TREE_SHA"
  prior_tag_object="$A16_TAG_OBJECT_SHA" prior_checksums="$A16_CHECKSUMS_SHA256" prior_amd64_archive="$A16_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A16_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A16_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A16_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A16_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A16_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A18_TAG" && "$COMPANION_VERSION" == "$A18_VERSION" ]]; then
  release_phase='A18' prior_phase='A17' prior_repository="$A17_REPOSITORY" prior_evidence_source='immutable A17 GitHub release' prior_tag="$A17_TAG" prior_version="$A17_VERSION" prior_commit="$A17_COMMIT_SHA" prior_release_id="$A17_RELEASE_ID" prior_tree="$A17_TREE_SHA"
  prior_tag_object="$A17_TAG_OBJECT_SHA" prior_checksums="$A17_CHECKSUMS_SHA256" prior_amd64_archive="$A17_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A17_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A17_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A17_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A17_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A17_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A19_TAG" && "$COMPANION_VERSION" == "$A19_VERSION" ]]; then
  release_phase='A19' prior_phase='A18' prior_repository="$A18_REPOSITORY" prior_evidence_source='immutable A18 GitHub release' prior_tag="$A18_TAG" prior_version="$A18_VERSION" prior_commit="$A18_COMMIT_SHA" prior_release_id="$A18_RELEASE_ID" prior_tree="$A18_TREE_SHA"
  prior_tag_object="$A18_TAG_OBJECT_SHA" prior_checksums="$A18_CHECKSUMS_SHA256" prior_amd64_archive="$A18_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A18_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A18_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A18_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A18_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A18_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A20_TAG" && "$COMPANION_VERSION" == "$A20_VERSION" ]]; then
  release_phase='A20' prior_phase='A19' prior_repository="$A19_REPOSITORY" prior_evidence_source='immutable A19 GitHub release' prior_tag="$A19_TAG" prior_version="$A19_VERSION" prior_commit="$A19_COMMIT_SHA" prior_release_id="$A19_RELEASE_ID" prior_tree="$A19_TREE_SHA"
  prior_tag_object="$A19_TAG_OBJECT_SHA" prior_checksums="$A19_CHECKSUMS_SHA256" prior_amd64_archive="$A19_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A19_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A19_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A19_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A19_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A19_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A21_TAG" && "$COMPANION_VERSION" == "$A21_VERSION" ]]; then
  release_phase='A21' prior_phase='A20' prior_repository="$A20_REPOSITORY" prior_evidence_source='immutable A20 GitHub release' prior_tag="$A20_TAG" prior_version="$A20_VERSION" prior_commit="$A20_COMMIT_SHA" prior_release_id="$A20_RELEASE_ID" prior_tree="$A20_TREE_SHA"
  prior_tag_object="$A20_TAG_OBJECT_SHA" prior_checksums="$A20_CHECKSUMS_SHA256" prior_amd64_archive="$A20_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A20_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A20_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A20_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A20_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A20_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A22_TAG" && "$COMPANION_VERSION" == "$A22_VERSION" ]]; then
  release_phase='A22' prior_phase='A21' prior_repository="$A21_REPOSITORY" prior_evidence_source='immutable A21 GitHub release' prior_tag="$A21_TAG" prior_version="$A21_VERSION" prior_commit="$A21_COMMIT_SHA" prior_release_id="$A21_RELEASE_ID" prior_tree="$A21_TREE_SHA"
  prior_tag_object="$A21_TAG_OBJECT_SHA" prior_checksums="$A21_CHECKSUMS_SHA256" prior_amd64_archive="$A21_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A21_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A21_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A21_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A21_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A21_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A23_TAG" && "$COMPANION_VERSION" == "$A23_VERSION" ]]; then
  release_phase='A23' prior_phase='A22' prior_repository="$A22_REPOSITORY" prior_evidence_source='immutable A22 GitHub release' prior_tag="$A22_TAG" prior_version="$A22_VERSION" prior_commit="$A22_COMMIT_SHA" prior_release_id="$A22_RELEASE_ID" prior_tree="$A22_TREE_SHA"
  prior_tag_object="$A22_TAG_OBJECT_SHA" prior_checksums="$A22_CHECKSUMS_SHA256" prior_amd64_archive='' prior_arm64_archive="$A22_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive='' prior_linux_arm64_archive='' prior_amd64_manifest='' prior_arm64_manifest="$A22_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A24_TAG" && "$COMPANION_VERSION" == "$A24_VERSION" ]]; then
  release_phase='A24' prior_phase='A23' prior_repository="$A23_REPOSITORY" prior_evidence_source='immutable A23 GitHub release' prior_tag="$A23_TAG" prior_version="$A23_VERSION" prior_commit="$A23_COMMIT_SHA" prior_release_id="$A23_RELEASE_ID" prior_tree="$A23_TREE_SHA"
  # A23 published no standalone companion manifest asset, but the manifest
  # travels inside both darwin archives, so the pins are measured from there.
  prior_tag_object="$A23_TAG_OBJECT_SHA" prior_checksums="$A23_CHECKSUMS_SHA256" prior_amd64_archive="$A23_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A23_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A23_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A23_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A23_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A23_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A25_TAG" && "$COMPANION_VERSION" == "$A25_VERSION" ]]; then
  release_phase='A25' prior_phase='A24' prior_repository="$A24_REPOSITORY" prior_evidence_source='immutable A24 GitHub release' prior_tag="$A24_TAG" prior_version="$A24_VERSION" prior_commit="$A24_COMMIT_SHA" prior_release_id="$A24_RELEASE_ID" prior_tree="$A24_TREE_SHA"
  # A24 shipped the same shape as A23: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A24_TAG_OBJECT_SHA" prior_checksums="$A24_CHECKSUMS_SHA256" prior_amd64_archive="$A24_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A24_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A24_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A24_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A24_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A24_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A26_TAG" && "$COMPANION_VERSION" == "$A26_VERSION" ]]; then
  release_phase='A26' prior_phase='A25' prior_repository="$A25_REPOSITORY" prior_evidence_source='immutable A25 GitHub release' prior_tag="$A25_TAG" prior_version="$A25_VERSION" prior_commit="$A25_COMMIT_SHA" prior_release_id="$A25_RELEASE_ID" prior_tree="$A25_TREE_SHA"
  # A25 shipped the same shape as A24: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A25_TAG_OBJECT_SHA" prior_checksums="$A25_CHECKSUMS_SHA256" prior_amd64_archive="$A25_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A25_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A25_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A25_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A25_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A25_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A27_TAG" && "$COMPANION_VERSION" == "$A27_VERSION" ]]; then
  release_phase='A27' prior_phase='A26' prior_repository="$A26_REPOSITORY" prior_evidence_source='immutable A26 GitHub release' prior_tag="$A26_TAG" prior_version="$A26_VERSION" prior_commit="$A26_COMMIT_SHA" prior_release_id="$A26_RELEASE_ID" prior_tree="$A26_TREE_SHA"
  # A26 shipped the same shape as A25: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A26_TAG_OBJECT_SHA" prior_checksums="$A26_CHECKSUMS_SHA256" prior_amd64_archive="$A26_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A26_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A26_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A26_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A26_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A26_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A28_TAG" && "$COMPANION_VERSION" == "$A28_VERSION" ]]; then
  release_phase='A28' prior_phase='A27' prior_repository="$A27_REPOSITORY" prior_evidence_source='immutable A27 GitHub release' prior_tag="$A27_TAG" prior_version="$A27_VERSION" prior_commit="$A27_COMMIT_SHA" prior_release_id="$A27_RELEASE_ID" prior_tree="$A27_TREE_SHA"
  # A27 shipped the same shape as A26: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A27_TAG_OBJECT_SHA" prior_checksums="$A27_CHECKSUMS_SHA256" prior_amd64_archive="$A27_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A27_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A27_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A27_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A27_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A27_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A29_TAG" && "$COMPANION_VERSION" == "$A29_VERSION" ]]; then
  release_phase='A29' prior_phase='A28' prior_repository="$A28_REPOSITORY" prior_evidence_source='immutable A28 GitHub release' prior_tag="$A28_TAG" prior_version="$A28_VERSION" prior_commit="$A28_COMMIT_SHA" prior_release_id="$A28_RELEASE_ID" prior_tree="$A28_TREE_SHA"
  # A28 shipped the same shape as A27: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A28_TAG_OBJECT_SHA" prior_checksums="$A28_CHECKSUMS_SHA256" prior_amd64_archive="$A28_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A28_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A28_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A28_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A28_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A28_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A30_TAG" && "$COMPANION_VERSION" == "$A30_VERSION" ]]; then
  release_phase='A30' prior_phase='A29' prior_repository="$A29_REPOSITORY" prior_evidence_source='immutable A29 GitHub release' prior_tag="$A29_TAG" prior_version="$A29_VERSION" prior_commit="$A29_COMMIT_SHA" prior_release_id="$A29_RELEASE_ID" prior_tree="$A29_TREE_SHA"
  # A29 shipped the same shape as A28: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A29_TAG_OBJECT_SHA" prior_checksums="$A29_CHECKSUMS_SHA256" prior_amd64_archive="$A29_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A29_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A29_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A29_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A29_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A29_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A31_TAG" && "$COMPANION_VERSION" == "$A31_VERSION" ]]; then
  release_phase='A31' prior_phase='A30' prior_repository="$A30_REPOSITORY" prior_evidence_source='immutable A30 GitHub release' prior_tag="$A30_TAG" prior_version="$A30_VERSION" prior_commit="$A30_COMMIT_SHA" prior_release_id="$A30_RELEASE_ID" prior_tree="$A30_TREE_SHA"
  # A30 shipped the same shape as A29: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A30_TAG_OBJECT_SHA" prior_checksums="$A30_CHECKSUMS_SHA256" prior_amd64_archive="$A30_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A30_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A30_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A30_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A30_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A30_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A32_TAG" && "$COMPANION_VERSION" == "$A32_VERSION" ]]; then
  release_phase='A32' prior_phase='A31' prior_repository="$A31_REPOSITORY" prior_evidence_source='immutable A31 GitHub release' prior_tag="$A31_TAG" prior_version="$A31_VERSION" prior_commit="$A31_COMMIT_SHA" prior_release_id="$A31_RELEASE_ID" prior_tree="$A31_TREE_SHA"
  # A31 shipped the same shape as A30: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A31_TAG_OBJECT_SHA" prior_checksums="$A31_CHECKSUMS_SHA256" prior_amd64_archive="$A31_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A31_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A31_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A31_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A31_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A31_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A33_TAG" && "$COMPANION_VERSION" == "$A33_VERSION" ]]; then
  release_phase='A33' prior_phase='A32' prior_repository="$A32_REPOSITORY" prior_evidence_source='immutable A32 GitHub release' prior_tag="$A32_TAG" prior_version="$A32_VERSION" prior_commit="$A32_COMMIT_SHA" prior_release_id="$A32_RELEASE_ID" prior_tree="$A32_TREE_SHA"
  # A32 shipped the same shape as A31: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A32_TAG_OBJECT_SHA" prior_checksums="$A32_CHECKSUMS_SHA256" prior_amd64_archive="$A32_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A32_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A32_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A32_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A32_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A32_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A34_TAG" && "$COMPANION_VERSION" == "$A34_VERSION" ]]; then
  release_phase='A34' prior_phase='A33' prior_repository="$A33_REPOSITORY" prior_evidence_source='immutable A33 GitHub release' prior_tag="$A33_TAG" prior_version="$A33_VERSION" prior_commit="$A33_COMMIT_SHA" prior_release_id="$A33_RELEASE_ID" prior_tree="$A33_TREE_SHA"
  # A33 shipped the same shape as A32: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A33_TAG_OBJECT_SHA" prior_checksums="$A33_CHECKSUMS_SHA256" prior_amd64_archive="$A33_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A33_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A33_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A33_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A33_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A33_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A35_TAG" && "$COMPANION_VERSION" == "$A35_VERSION" ]]; then
  release_phase='A35' prior_phase='A34' prior_repository="$A34_REPOSITORY" prior_evidence_source='immutable A34 GitHub release' prior_tag="$A34_TAG" prior_version="$A34_VERSION" prior_commit="$A34_COMMIT_SHA" prior_release_id="$A34_RELEASE_ID" prior_tree="$A34_TREE_SHA"
  # A34 shipped the same shape as A33: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A34_TAG_OBJECT_SHA" prior_checksums="$A34_CHECKSUMS_SHA256" prior_amd64_archive="$A34_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A34_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A34_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A34_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A34_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A34_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A36_TAG" && "$COMPANION_VERSION" == "$A36_VERSION" ]]; then
  release_phase='A36' prior_phase='A35' prior_repository="$A35_REPOSITORY" prior_evidence_source='immutable A35 GitHub release' prior_tag="$A35_TAG" prior_version="$A35_VERSION" prior_commit="$A35_COMMIT_SHA" prior_release_id="$A35_RELEASE_ID" prior_tree="$A35_TREE_SHA"
  # A35 shipped the same shape as A34: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A35_TAG_OBJECT_SHA" prior_checksums="$A35_CHECKSUMS_SHA256" prior_amd64_archive="$A35_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A35_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A35_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A35_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A35_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A35_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A37_TAG" && "$COMPANION_VERSION" == "$A37_VERSION" ]]; then
  release_phase='A37' prior_phase='A36' prior_repository="$A36_REPOSITORY" prior_evidence_source='immutable A36 GitHub release' prior_tag="$A36_TAG" prior_version="$A36_VERSION" prior_commit="$A36_COMMIT_SHA" prior_release_id="$A36_RELEASE_ID" prior_tree="$A36_TREE_SHA"
  # A36 shipped the same shape as A35: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A36_TAG_OBJECT_SHA" prior_checksums="$A36_CHECKSUMS_SHA256" prior_amd64_archive="$A36_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A36_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A36_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A36_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A36_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A36_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A38_TAG" && "$COMPANION_VERSION" == "$A38_VERSION" ]]; then
  release_phase='A38' prior_phase='A37' prior_repository="$A37_REPOSITORY" prior_evidence_source='immutable A37 GitHub release' prior_tag="$A37_TAG" prior_version="$A37_VERSION" prior_commit="$A37_COMMIT_SHA" prior_release_id="$A37_RELEASE_ID" prior_tree="$A37_TREE_SHA"
  # A37 shipped the same shape as A36: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A37_TAG_OBJECT_SHA" prior_checksums="$A37_CHECKSUMS_SHA256" prior_amd64_archive="$A37_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A37_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A37_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A37_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A37_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A37_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A39_TAG" && "$COMPANION_VERSION" == "$A39_VERSION" ]]; then
  release_phase='A39' prior_phase='A38' prior_repository="$A38_REPOSITORY" prior_evidence_source='immutable A38 GitHub release' prior_tag="$A38_TAG" prior_version="$A38_VERSION" prior_commit="$A38_COMMIT_SHA" prior_release_id="$A38_RELEASE_ID" prior_tree="$A38_TREE_SHA"
  # A38 shipped the same shape as A37: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A38_TAG_OBJECT_SHA" prior_checksums="$A38_CHECKSUMS_SHA256" prior_amd64_archive="$A38_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A38_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A38_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A38_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A38_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A38_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A40_TAG" && "$COMPANION_VERSION" == "$A40_VERSION" ]]; then
  release_phase='A40' prior_phase='A39' prior_repository="$A39_REPOSITORY" prior_evidence_source='immutable A39 GitHub release' prior_tag="$A39_TAG" prior_version="$A39_VERSION" prior_commit="$A39_COMMIT_SHA" prior_release_id="$A39_RELEASE_ID" prior_tree="$A39_TREE_SHA"
  # A39 shipped the same shape as A38: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A39_TAG_OBJECT_SHA" prior_checksums="$A39_CHECKSUMS_SHA256" prior_amd64_archive="$A39_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A39_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A39_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A39_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A39_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A39_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A41_TAG" && "$COMPANION_VERSION" == "$A41_VERSION" ]]; then
  release_phase='A41' prior_phase='A40' prior_repository="$A40_REPOSITORY" prior_evidence_source='immutable A40 GitHub release' prior_tag="$A40_TAG" prior_version="$A40_VERSION" prior_commit="$A40_COMMIT_SHA" prior_release_id="$A40_RELEASE_ID" prior_tree="$A40_TREE_SHA"
  # A40 shipped the same shape as A39: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A40_TAG_OBJECT_SHA" prior_checksums="$A40_CHECKSUMS_SHA256" prior_amd64_archive="$A40_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A40_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A40_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A40_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A40_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A40_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A42_TAG" && "$COMPANION_VERSION" == "$A42_VERSION" ]]; then
  release_phase='A42' prior_phase='A41' prior_repository="$A41_REPOSITORY" prior_evidence_source='immutable A41 GitHub release' prior_tag="$A41_TAG" prior_version="$A41_VERSION" prior_commit="$A41_COMMIT_SHA" prior_release_id="$A41_RELEASE_ID" prior_tree="$A41_TREE_SHA"
  # A41 shipped the same shape as A40: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A41_TAG_OBJECT_SHA" prior_checksums="$A41_CHECKSUMS_SHA256" prior_amd64_archive="$A41_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A41_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A41_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A41_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A41_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A41_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A43_TAG" && "$COMPANION_VERSION" == "$A43_VERSION" ]]; then
  release_phase='A43' prior_phase='A42' prior_repository="$A42_REPOSITORY" prior_evidence_source='immutable A42 GitHub release' prior_tag="$A42_TAG" prior_version="$A42_VERSION" prior_commit="$A42_COMMIT_SHA" prior_release_id="$A42_RELEASE_ID" prior_tree="$A42_TREE_SHA"
  # A42 shipped the same shape as A41: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A42_TAG_OBJECT_SHA" prior_checksums="$A42_CHECKSUMS_SHA256" prior_amd64_archive="$A42_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A42_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A42_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A42_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A42_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A42_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A44_TAG" && "$COMPANION_VERSION" == "$A44_VERSION" ]]; then
  release_phase='A44' prior_phase='A43' prior_repository="$A43_REPOSITORY" prior_evidence_source='immutable A43 GitHub release' prior_tag="$A43_TAG" prior_version="$A43_VERSION" prior_commit="$A43_COMMIT_SHA" prior_release_id="$A43_RELEASE_ID" prior_tree="$A43_TREE_SHA"
  # A43 shipped the same shape as A42: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A43_TAG_OBJECT_SHA" prior_checksums="$A43_CHECKSUMS_SHA256" prior_amd64_archive="$A43_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A43_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A43_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A43_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A43_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A43_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A45_TAG" && "$COMPANION_VERSION" == "$A45_VERSION" ]]; then
  release_phase='A45' prior_phase='A44' prior_repository="$A44_REPOSITORY" prior_evidence_source='immutable A44 GitHub release' prior_tag="$A44_TAG" prior_version="$A44_VERSION" prior_commit="$A44_COMMIT_SHA" prior_release_id="$A44_RELEASE_ID" prior_tree="$A44_TREE_SHA"
  # A44 shipped the same shape as A43: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A44_TAG_OBJECT_SHA" prior_checksums="$A44_CHECKSUMS_SHA256" prior_amd64_archive="$A44_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A44_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A44_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A44_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A44_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A44_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A46_TAG" && "$COMPANION_VERSION" == "$A46_VERSION" ]]; then
  release_phase='A46' prior_phase='A45' prior_repository="$A45_REPOSITORY" prior_evidence_source='immutable A45 GitHub release' prior_tag="$A45_TAG" prior_version="$A45_VERSION" prior_commit="$A45_COMMIT_SHA" prior_release_id="$A45_RELEASE_ID" prior_tree="$A45_TREE_SHA"
  # A45 shipped the same shape as A44: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A45_TAG_OBJECT_SHA" prior_checksums="$A45_CHECKSUMS_SHA256" prior_amd64_archive="$A45_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A45_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A45_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A45_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A45_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A45_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A47_TAG" && "$COMPANION_VERSION" == "$A47_VERSION" ]]; then
  release_phase='A47' prior_phase='A46' prior_repository="$A46_REPOSITORY" prior_evidence_source='immutable A46 GitHub release' prior_tag="$A46_TAG" prior_version="$A46_VERSION" prior_commit="$A46_COMMIT_SHA" prior_release_id="$A46_RELEASE_ID" prior_tree="$A46_TREE_SHA"
  # A46 shipped the same shape as A45: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A46_TAG_OBJECT_SHA" prior_checksums="$A46_CHECKSUMS_SHA256" prior_amd64_archive="$A46_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A46_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A46_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A46_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A46_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A46_ARM64_MANIFEST_SHA256"
elif [[ "$GITHUB_REF_NAME" == "$A48_TAG" && "$COMPANION_VERSION" == "$A48_VERSION" ]]; then
  release_phase='A48' prior_phase='A47' prior_repository="$A47_REPOSITORY" prior_evidence_source='immutable A47 GitHub release' prior_tag="$A47_TAG" prior_version="$A47_VERSION" prior_commit="$A47_COMMIT_SHA" prior_release_id="$A47_RELEASE_ID" prior_tree="$A47_TREE_SHA"
  # A47 shipped the same shape as A46: four archives, checksums, and the
  # companion manifest inside each darwin bundle.
  prior_tag_object="$A47_TAG_OBJECT_SHA" prior_checksums="$A47_CHECKSUMS_SHA256" prior_amd64_archive="$A47_AMD64_ARCHIVE_SHA256" prior_arm64_archive="$A47_ARM64_ARCHIVE_SHA256" prior_linux_amd64_archive="$A47_LINUX_AMD64_ARCHIVE_SHA256" prior_linux_arm64_archive="$A47_LINUX_ARM64_ARCHIVE_SHA256" prior_amd64_manifest="$A47_AMD64_MANIFEST_SHA256" prior_arm64_manifest="$A47_ARM64_MANIFEST_SHA256"
else
  fail prior_release_identity_mismatch 'release is outside the frozen A0/A1/A2/A3/A4/A5/A6/A7/A8/A9/A10/A11/A12/A13/A14/A15/A16/A17/A18/A19/A20/A21/A22/A23/A24/A25/A26/A27/A28/A29/A30/A31/A32/A33/A34/A35/A36/A37/A38/A39/A40/A41/A42/A43/A44/A45/A46/A47/A48 policy'
fi
