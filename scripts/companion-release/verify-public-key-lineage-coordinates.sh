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
# The A23 through A27 predecessor pins follow A22's in
# verify-public-key-lineage-pins.sh, which the lineage verifier sources first.
# They moved there unchanged, block by block, as this file reached the
# 300-line source limit.
readonly A24_REPOSITORY='Insajin/autopus-adk' A25_TAG='v0.50.114' A25_VERSION='0.50.114'
readonly A25_REPOSITORY='Insajin/autopus-adk' A26_TAG='v0.50.115' A26_VERSION='0.50.115'
readonly A26_REPOSITORY='Insajin/autopus-adk' A27_TAG='v0.50.116' A27_VERSION='0.50.116'
readonly A27_REPOSITORY='Insajin/autopus-adk' A28_TAG='v0.50.117' A28_VERSION='0.50.117'
readonly A28_REPOSITORY='Insajin/autopus-adk' A29_TAG='v0.50.118' A29_VERSION='0.50.118'
# A28 published the same fifteen-asset set as A27, all measured from the
# immutable release 383826825. The two manifest digests come from inside the
# darwin archives, where the companion manifest travels.
readonly A28_RELEASE_ID='383826825'
readonly A28_COMMIT_SHA='620e29a44d004cb199d5f1c22ae92878f9b6930e'
readonly A28_TREE_SHA='fce0d047ae8cf5c519762fe4ebf530e7ccc01ed1'
readonly A28_TAG_OBJECT_SHA='edcd4eb98f4e52e8ff868d1f8440cfa0d0c5bc3a'
readonly A28_CHECKSUMS_SHA256='1a50d39789026708cea1f4d0352f14c244b59bbeddb0afaa52463970ce2684ef'
readonly A28_AMD64_ARCHIVE_SHA256='0f0fea1f7f16f61f049ebf2ad6d9af789ad8380187d85718ff75d031c538432a'
readonly A28_ARM64_ARCHIVE_SHA256='3a2dcdf7d0e89ca93dae32f6560b096f3c20903f8a7a7a8407a53f017c50261c'
readonly A28_LINUX_AMD64_ARCHIVE_SHA256='f44896aef208b2dc819355a7cd89ec3d62c58bf26fe8bccf06528d7da1a08117'
readonly A28_LINUX_ARM64_ARCHIVE_SHA256='f084d2a4526c295d876b46db3638101a79f44a7344843d487a8db3a83f12e9f7'
readonly A28_AMD64_MANIFEST_SHA256='9d7e50b0303048707ebde925cfdc2cad95ba411b9069b3832198d6cd7a1b76f4'
readonly A28_ARM64_MANIFEST_SHA256='78d8d2064ef11f69eab9c29c809d0441f7034db32927415d690cb58bb6c1578c'
readonly A29_REPOSITORY='autopus-ai/autopus-adk' A30_TAG='v0.50.119' A30_VERSION='0.50.119'
# A29 is the first release published after the transfer to autopus-ai, so its
# evidence lives under the new owner. It shipped the same fifteen-asset set as
# A28, all measured from the immutable release 392314996. The two manifest
# digests come from inside the darwin archives, where the companion manifest
# travels.
readonly A29_RELEASE_ID='392314996'
readonly A29_COMMIT_SHA='4480c8d2f6c00c205ee838cd4bd20933bfff3597'
readonly A29_TREE_SHA='84dc6034e9062428e5347fbc6f9827d34f847cf9'
readonly A29_TAG_OBJECT_SHA='e51f2ead87a39d499733075fa755fc04033d5ac2'
readonly A29_CHECKSUMS_SHA256='e3526a66a559abf4842c4cd44666c2c68985df6c6013f5b7f9b7e4316bf5a4a6'
readonly A29_AMD64_ARCHIVE_SHA256='5ec4a60db05e1af3b144dda943fc5221c203ad3b0a008020d990df63985dab63'
readonly A29_ARM64_ARCHIVE_SHA256='896cc412efcab42b25d88559497f958e861d801da466afac11bed722b014160c'
readonly A29_LINUX_AMD64_ARCHIVE_SHA256='cb3c6a4241892079baba5c932290a05f67a08a06f27e41f675d90bab1c83b09b'
readonly A29_LINUX_ARM64_ARCHIVE_SHA256='09d3817e969ae61a9b314d50eb15f5148b07eb6f85fb89d2350c1275d7b87b35'
readonly A29_AMD64_MANIFEST_SHA256='07621b97f097feb4a2b5057be845e282bfa1e49ecd3976648872b7b346f2198f'
readonly A29_ARM64_MANIFEST_SHA256='bc235b9aca1fd91ab6077f88c6114f2b129a9e96faaabd76b58b65430f4b2a88'
readonly A30_REPOSITORY='autopus-ai/autopus-adk' A31_TAG='v0.50.120' A31_VERSION='0.50.120'
# A30 shipped the same fifteen-asset set as A29 under the same owner, all
# measured from the immutable release 398256974. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A30_RELEASE_ID='398256974'
readonly A30_COMMIT_SHA='279bc98635639a91e08285c5ffc649d8f4c7df26'
readonly A30_TREE_SHA='3c4fc4d70bd13f4693ca31e1d272fa872971b7ed'
readonly A30_TAG_OBJECT_SHA='cffd4aa74b741f471e576047ae0d53d8db94c7d5'
readonly A30_CHECKSUMS_SHA256='b095aac56f0036df893d5bb3b67d61fb67f0640dc10b0694ca404c520dd1442a'
readonly A30_AMD64_ARCHIVE_SHA256='23b1d6c71fd81d1ecb36b86bb44aec8a0609ee40b10ed49b84a69e699647dcfe'
readonly A30_ARM64_ARCHIVE_SHA256='b58a0ceec913db6974a79bd8050642daebe9149ee0d8c3a73d2b8543125d1aae'
readonly A30_LINUX_AMD64_ARCHIVE_SHA256='7e4b3f42aa6afe604388ddeb3857d0a023e93fd406a7000c23a374cdcc77bad3'
readonly A30_LINUX_ARM64_ARCHIVE_SHA256='79fc2997330901339690d9508c34c52739536aeffbbaba6f2672f84aa96fad3d'
readonly A30_AMD64_MANIFEST_SHA256='3651a4e44ed7a7850450990ba4edd85e42766ebec0a81f2e490d0710c86ba1a2'
readonly A30_ARM64_MANIFEST_SHA256='79a09f39994f633b26e83f6ab1337f4f47b8da94f0d24f01e02d0e56bdbb739f'
readonly A31_REPOSITORY='autopus-ai/autopus-adk' A32_TAG='v0.50.121' A32_VERSION='0.50.121'
# A31 shipped the same fifteen-asset set as A30 under the same owner, all
# measured from the immutable release 398906327. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A31_RELEASE_ID='398906327'
readonly A31_COMMIT_SHA='b69a8450d6a1c916dfd6e86b254907b4b0c76b78'
readonly A31_TREE_SHA='9e003120060540c4f361c81d10242970ef7c350b'
readonly A31_TAG_OBJECT_SHA='cee58b628ee8fe83c449a7d2ed46a2185168a4eb'
readonly A31_CHECKSUMS_SHA256='9fe21febfccbb5af51f8dfc3bc229d023278b1051e2d5ea98d02f2c324039e50'
readonly A31_AMD64_ARCHIVE_SHA256='b5a78f7afb4b9ea7e3439e02f7d0dad66dd0d8af2e4e90c9510b7c341ecb1d34'
readonly A31_ARM64_ARCHIVE_SHA256='2359b595f4fb3857b2e8c0b9ed849df3445586b7c6bd680367d610e2c71a0cdc'
readonly A31_LINUX_AMD64_ARCHIVE_SHA256='fd2551898babf2c00fe5ec84146876e7750074eb65e878521fb6c770f30cf1e5'
readonly A31_LINUX_ARM64_ARCHIVE_SHA256='3120e5acd77e996df534b85d6502139d60e4a3070d482e85044f9e2071b1e6db'
readonly A31_AMD64_MANIFEST_SHA256='7af79e2c3849972f493de69fd563e234d5b147c551f195291a9f10ac9ea5781c'
readonly A31_ARM64_MANIFEST_SHA256='e321e8556853ff2c0d986cbff385b520fd9602e8722ca4dc6b90261cce240c61'
readonly A32_REPOSITORY='autopus-ai/autopus-adk' A33_TAG='v0.50.122' A33_VERSION='0.50.122'
# A32 shipped the same fifteen-asset set as A31 under the same owner, all
# measured from the immutable release 402213224. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A32_RELEASE_ID='402213224'
readonly A32_COMMIT_SHA='8299c8df85bc0e24bfb4d4fbbd71889dceb96986'
readonly A32_TREE_SHA='2a73b9db4adbaab78c29c37a75e3be8ffa4f365d'
readonly A32_TAG_OBJECT_SHA='033a3581a00c3566e3d19841fad9cf495c20cfb3'
readonly A32_CHECKSUMS_SHA256='5c845085e078d0b211335d350b83d2d2fba8d9121ce86c158043921f60e85826'
readonly A32_AMD64_ARCHIVE_SHA256='6df337b4ff4efb01f4070e941e31df37bb81f647e75b230b826e9ae5c0fd7661'
readonly A32_ARM64_ARCHIVE_SHA256='de90e3165a504c8c8dbe128114c5f2b9fc9c72890d12d1cc3695c3aea3e7dd17'
readonly A32_LINUX_AMD64_ARCHIVE_SHA256='ddf313f8b0e0b297086b5b95458d9fa371e921792eff42d5a22bf838a9294e34'
readonly A32_LINUX_ARM64_ARCHIVE_SHA256='2684b264b9fd95e77001dd77afb6bccb9b474a7bb03a06311d0e802a2b0bab0a'
readonly A32_AMD64_MANIFEST_SHA256='055b8e868d9f2dc94659322aeff227686dbdd56367cfaa93b1fc870262ae90e2'
readonly A32_ARM64_MANIFEST_SHA256='7109e5c358a50735ed125ee97715d1576ecf92e40ec8b685139d289552570679'
readonly A33_REPOSITORY='autopus-ai/autopus-adk' A34_TAG='v0.50.123' A34_VERSION='0.50.123'
# A33 shipped the same fifteen-asset set as A32 under the same owner, all
# measured from the immutable release 402813712. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A33_RELEASE_ID='402813712'
readonly A33_COMMIT_SHA='c42337f4dcf9592d065130150af18dcc5a16c5b8'
readonly A33_TREE_SHA='dad6a86e1e493988bc77b8705b3c00c4dd7576a1'
readonly A33_TAG_OBJECT_SHA='da8d4f31267bd7be0999b232e73623cd2e6f6034'
readonly A33_CHECKSUMS_SHA256='04ff98b075dd9f314069fb8e42b45c024cc54017f073ccc681dae550f48c5a2f'
readonly A33_AMD64_ARCHIVE_SHA256='2b6e88c5f64d5c19d28e4d1fdbec15a0aea49031afcded2c28a076fc60b4038e'
readonly A33_ARM64_ARCHIVE_SHA256='308072559eaaff75f22c1cae11e2939f1672b341ae5957acc22459ed300f882e'
readonly A33_LINUX_AMD64_ARCHIVE_SHA256='205f17bbafc9f55ad79243feed67de07ad76c3186cfcf272b3321e5b95218f33'
readonly A33_LINUX_ARM64_ARCHIVE_SHA256='49d7cdf6bb1c8685676d1f5805ef945960cb45958e33eb605bbc3815e3cbef3d'
readonly A33_AMD64_MANIFEST_SHA256='57317da672620d4f5c7ca58358fea8d0c8e87459f5030c8c337a58d872f7fed5'
readonly A33_ARM64_MANIFEST_SHA256='491a7576c3b3febb03cb0465da99c9b953d2c5fd6edfa725ce393ced807c3fa0'
readonly A34_REPOSITORY='autopus-ai/autopus-adk' A35_TAG='v0.50.125' A35_VERSION='0.50.125'
# A34 shipped the same fifteen-asset set as A33 under the same owner, all
# measured from the immutable release 402971620. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A34_RELEASE_ID='402971620'
readonly A34_COMMIT_SHA='c447badc28e393b19984d2eeea159a81d609acd9'
readonly A34_TREE_SHA='13dcb08d81c59ad0677e824b0785bc7e1fffcbe7'
readonly A34_TAG_OBJECT_SHA='8ac711f82b6879b4bbf582de88658315b037a1b7'
readonly A34_CHECKSUMS_SHA256='f15071f66e5056e5e483a03c85f49ebe1fc4e3cc35df24368ca3ca3f35213538'
readonly A34_AMD64_ARCHIVE_SHA256='74c48a1686ecbeee155bfdaa002a4f1f2e553d222ec351e196507f7f470be071'
readonly A34_ARM64_ARCHIVE_SHA256='4c8809fda4bede93601711892fc91cbcc334cf59a7e0200848b3d4d9b16b2d94'
readonly A34_LINUX_AMD64_ARCHIVE_SHA256='864ee59cfbb076941a5c4bc0839b12fb0c7c2ccd6bd99718ba54774dad81cc6f'
readonly A34_LINUX_ARM64_ARCHIVE_SHA256='c62d0e10714600d9ce4e9bbdfee668c1e1f828c98489213b1ba8c7f4aed75249'
readonly A34_AMD64_MANIFEST_SHA256='b45f736729725e62a54cec87dce7841723d7a7aec6269345f2ff05849cd8190d'
readonly A34_ARM64_MANIFEST_SHA256='b7973fe43ae90398dd14bbc8370822d21be2dbb15df2be31ab760b0abdfa8757'
readonly A35_REPOSITORY='autopus-ai/autopus-adk' A36_TAG='v0.50.127' A36_VERSION='0.50.127'
# A35 shipped the same fifteen-asset set as A34 under the same owner, all
# measured from the immutable release 410986343. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A35_RELEASE_ID='410986343'
readonly A35_COMMIT_SHA='dd6d6ea2435d759144ec093b9b464a831f47703d'
readonly A35_TREE_SHA='8d6aa0c4cf84f2c806205d3210505d100b863233'
readonly A35_TAG_OBJECT_SHA='0616d14060c9bbd53d5196faeaa0bc8e108edb2b'
readonly A35_CHECKSUMS_SHA256='5f2bf32db0d96266a7fc84b9bc652163fe16184ce2a8aaeefbedb637d22e0a48'
readonly A35_AMD64_ARCHIVE_SHA256='52538f80e3f88c3c530e279ab72ed780c33da7045b3b175337ed6a28f4672513'
readonly A35_ARM64_ARCHIVE_SHA256='5253996a619c350348870fb240301833e69803b2a2ea5e430c0663ab2aa84747'
readonly A35_LINUX_AMD64_ARCHIVE_SHA256='0dc07d1e74889dc2b3fd511cd4d3c4a470e34b7d26a30b7f55cdbb08df86ae44'
readonly A35_LINUX_ARM64_ARCHIVE_SHA256='67c6c9f379b782bb0cea7a39c7a42a6372995e88c8dfbd295e8cfbeb32ee8363'
readonly A35_AMD64_MANIFEST_SHA256='5a4e5cdc55b479eb1fd1942c5885ba2f4599ed9fe9151aebb1c44b95640e1b98'
readonly A35_ARM64_MANIFEST_SHA256='79acf762c930a7ad7a1f19f2bf79b2e1dacc31daa8255515f6cd20c2761f0836'
readonly A36_REPOSITORY='autopus-ai/autopus-adk' A37_TAG='v0.50.128' A37_VERSION='0.50.128'
# A36 shipped the same fifteen-asset set as A35 under the same owner, all
# measured from the immutable release 421737046. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A36_RELEASE_ID='421737046'
readonly A36_COMMIT_SHA='81186d0c73bb6eb633e52a7cb6a2af23d0f54d12'
readonly A36_TREE_SHA='7f66f3b2394f0df4c696e961cbd70f4be754b6f6'
readonly A36_TAG_OBJECT_SHA='ac00440870377496345d4b104e8554aef7f62c09'
readonly A36_CHECKSUMS_SHA256='19234799128ef9e6cf90be1fa00519ac86c02b4df393f6eeff6185256b77d265'
readonly A36_AMD64_ARCHIVE_SHA256='c45588b9d88362632646039de8b5475823e8164fb8bd86694c134654906cc42d'
readonly A36_ARM64_ARCHIVE_SHA256='a37e263dafe09966ef25e2705065a4ff04ad781a43fe8db8754daf20b8f702d2'
readonly A36_LINUX_AMD64_ARCHIVE_SHA256='4ac9204d6087ea63b03b525f7bb4e9f0a0686d6df11174e7eab56f8c16a4c78d'
readonly A36_LINUX_ARM64_ARCHIVE_SHA256='72aeab8e2f7a49bcc2a028d652281d4588637feb6975fad79b6147c063b9d512'
readonly A36_AMD64_MANIFEST_SHA256='0db6e95b614709e844800ca00443feeabca56facb433e0c331b675ac918e2041'
readonly A36_ARM64_MANIFEST_SHA256='1252174cf8b2517d9bbc7cd89f70b5067eb3814d777a44128741b3fbc962f082'
readonly A37_REPOSITORY='autopus-ai/autopus-adk' A38_TAG='v0.50.131' A38_VERSION='0.50.131'
# A37 shipped the same fifteen-asset set as A36 under the same owner, all
# measured from the immutable release 427217669. The two manifest digests come
# from inside the darwin archives, where the companion manifest travels.
readonly A37_RELEASE_ID='427217669'
readonly A37_COMMIT_SHA='842f8bf286aa4c26554cc0d29f3c79c412069a0b'
readonly A37_TREE_SHA='a13f508cd95e6a902b3353d8e08c06dedbfae809'
readonly A37_TAG_OBJECT_SHA='04fd540532546f2651e63c0de007fbbac1ea3b93'
readonly A37_CHECKSUMS_SHA256='f8d28b4961e79f54470ddd234dbfa3827b897ce57bc0c64d1da4293b11e4543e'
readonly A37_AMD64_ARCHIVE_SHA256='f4bf3bdbd1f284d327801c5fd8693d4396b696bd2e83ef1331ee40e5456be605'
readonly A37_ARM64_ARCHIVE_SHA256='6f455de994198a21b0e36abb71fde92f5aa54905418b6bd4942c8b339d227fe2'
readonly A37_LINUX_AMD64_ARCHIVE_SHA256='9b96e505b019dba5f26501c92a7053d9f175d1eb2cbd7dd410254ca00cc9003f'
readonly A37_LINUX_ARM64_ARCHIVE_SHA256='d555b4e0e74b981c0521e50e47a2dda7e1d9952dcf99130a342dbe40d8785845'
readonly A37_AMD64_MANIFEST_SHA256='6dc134938b0c88bdfa52e374dc27ff4185d5bd8dc9597fc4f8cb1c7a20e0a73c'
readonly A37_ARM64_MANIFEST_SHA256='2be72a5784bdee43874d41ae69bf29c28bf07f38f9b11afb85092304496953d3'
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
else
  fail prior_release_identity_mismatch 'release is outside the frozen A0/A1/A2/A3/A4/A5/A6/A7/A8/A9/A10/A11/A12/A13/A14/A15/A16/A17/A18/A19/A20/A21/A22/A23/A24/A25/A26/A27/A28/A29/A30/A31/A32/A33/A34/A35/A36/A37/A38 policy'
fi
