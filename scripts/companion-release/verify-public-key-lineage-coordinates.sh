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
# A23 published a full asset set, so A24 pins every archive rather than the
# arm64-only subset A22's bridge could offer. Read from release 379595447.
readonly A23_RELEASE_ID='379595447'
readonly A23_COMMIT_SHA='954f60a77acb59fd4106537020693fdcadb3d640'
readonly A23_TREE_SHA='fcd3f2aed498955235ae7807ba031d32a053db09'
readonly A23_TAG_OBJECT_SHA='b751c5beba4374534b1a73615ff0d6d57bdb4131'
readonly A23_CHECKSUMS_SHA256='9d315535abe8b67779c7f5fe598aece7b8b60d48b9aa5c8c55b01e02e03a86f2'
readonly A23_AMD64_ARCHIVE_SHA256='366d3329081649f322ca29a1e33406c7bcf9f08bd490ec740306b5c8bcc12982'
readonly A23_ARM64_ARCHIVE_SHA256='a0a06284a86dfaf2175b9c8114dc6f5c72bdf4553637605455b44f85cf59973b'
readonly A23_LINUX_AMD64_ARCHIVE_SHA256='62e979308e09b28fa9976fd65172ed8483ed30b6bd0adcb68071c6fd61853207'
readonly A23_LINUX_ARM64_ARCHIVE_SHA256='006cc1b8bcd9f4bbbbdaf051a1cef62fb1578747c170a9019934b43d33c495cf'
# Measured from the two immutable darwin archives of release 379595447: the
# companion manifest travels inside each bundle, so A24 can pin it exactly
# rather than leaving the pin empty and skipping the binding check.
readonly A23_AMD64_MANIFEST_SHA256='7c60a437970e7e7c157386c883ea47cadffbc559af12311726473879b34a4325'
readonly A23_ARM64_MANIFEST_SHA256='b2a6c5c98a9691ee482748ec7946b0dcd0d10cb7d5c0e11eb83720ca40bd7c3d'
readonly A24_REPOSITORY='Insajin/autopus-adk' A25_TAG='v0.50.114' A25_VERSION='0.50.114'
# A24 published the same fifteen-asset set as A23, all measured from the
# immutable release 381657693. The two manifest digests come from inside the
# darwin archives, where the companion manifest travels.
readonly A24_RELEASE_ID='381657693'
readonly A24_COMMIT_SHA='bc2147a875b49e9fca75db4307455f83512837d6'
readonly A24_TREE_SHA='35d1134cc4197baca4b373d9102c780565346f92'
readonly A24_TAG_OBJECT_SHA='158269c057e3e45f0a2d5353a1fb2992878bc9f3'
readonly A24_CHECKSUMS_SHA256='558b9c7e0a8806c890abea8cc053325a15afd46deba5edf20b0f2e2ce89415b7'
readonly A24_AMD64_ARCHIVE_SHA256='3ac160607bf77bf762b7101d5768b72c6909b72601902554dcd35f39363e2e64'
readonly A24_ARM64_ARCHIVE_SHA256='d82c75b347f78a0370bfa330f5da4e075e54474aa6535f69d5ee72bf1bfce64b'
readonly A24_LINUX_AMD64_ARCHIVE_SHA256='4354d4f9153d8a7567cf5c7fd03bce0344b9474bed5488a0183d3c5755d6b482'
readonly A24_LINUX_ARM64_ARCHIVE_SHA256='09d6454488109071d20e29fe44be49751900d895d4bc03f5695bc83a26a033a5'
readonly A24_AMD64_MANIFEST_SHA256='1d5e74fa32f6a490e55c761a495254d8fb82ba05ff65619f1c2b0d85ef3343df'
readonly A24_ARM64_MANIFEST_SHA256='22340ec288ebed9a82c53e3550dc618a241c6c31bccedbe09a022b5e3df284cd'
readonly A25_REPOSITORY='Insajin/autopus-adk' A26_TAG='v0.50.115' A26_VERSION='0.50.115'
# A25 published the same fifteen-asset set as A24, all measured from the
# immutable release 382345734. The two manifest digests come from inside the
# darwin archives, where the companion manifest travels.
readonly A25_RELEASE_ID='382345734'
readonly A25_COMMIT_SHA='a6d199fb5a7b27721026916fcd75dffb58a4e228'
readonly A25_TREE_SHA='61dad3b64bc180a7582f967b47da98762c979b3c'
readonly A25_TAG_OBJECT_SHA='7667c522042e58914f90919a5a283cef1acd533f'
readonly A25_CHECKSUMS_SHA256='6624f9001979f4b45732f17b5ee06f57dcb83f110df6f1df55a81b46be6e9f48'
readonly A25_AMD64_ARCHIVE_SHA256='0c89ee54327047a7234c822d27158e8577ddb94a5dad2b4fc5339a0ad7e21eee'
readonly A25_ARM64_ARCHIVE_SHA256='acc19a15ba2f72a9cfaeadbe160fe72579924befa5eaaa44b8ebfcf0193c7239'
readonly A25_LINUX_AMD64_ARCHIVE_SHA256='a8c5d811eafc1185bdfb55d53360330ef94a36ce04a9e535c79f90c804edf49c'
readonly A25_LINUX_ARM64_ARCHIVE_SHA256='a0bfb36c02e96a2a1308da1c4d07a2c7a247054ddc0866acb496bb1f00c16162'
readonly A25_AMD64_MANIFEST_SHA256='6be74165a9edcb7cd1158342d97fdaf72ac4b4623d27e4088539e00061bc8744'
readonly A25_ARM64_MANIFEST_SHA256='a17d1bfa230473770df19d93d9c9ca8e17318f755cb1c1fe49d5df8b6536977f'
readonly A26_REPOSITORY='Insajin/autopus-adk' A27_TAG='v0.50.116' A27_VERSION='0.50.116'
# A26 published the same fifteen-asset set as A25, all measured from the
# immutable release 383249963. The two manifest digests come from inside the
# darwin archives, where the companion manifest travels.
readonly A26_RELEASE_ID='383249963'
readonly A26_COMMIT_SHA='77ae668bf7e9eb8d0dae177d1c9b7e41a5d51ef6'
readonly A26_TREE_SHA='e353a4bc1c3e3cca217d40751095fdc8b9a3a3da'
readonly A26_TAG_OBJECT_SHA='058f0fd92cc9c73ea48f376a5ffdd2883ca11b99'
readonly A26_CHECKSUMS_SHA256='7b6bf3c5f3dd461435a0c434c696b9bf683f643ab64f5d0bca78e9d9857a0005'
readonly A26_AMD64_ARCHIVE_SHA256='00d59caf1d76d2cc74866e3be14e6fae5c07b03365a782946121ffaafc557a1f'
readonly A26_ARM64_ARCHIVE_SHA256='7765aa3ecad8968e2c7cf06c73472d0b423a9a31e7baf33c4ffa488cfa35cc45'
readonly A26_LINUX_AMD64_ARCHIVE_SHA256='450de922ed7e0630b1ceabd8592e3b4f9ca16fa6041d295aaff0c9f33748732a'
readonly A26_LINUX_ARM64_ARCHIVE_SHA256='fcf262b010d3c02e994a3724a9b2e5754a2ad6debc315d27f3f11d8ee2d392b2'
readonly A26_AMD64_MANIFEST_SHA256='49b9bdf5bf046306ee952ac86e443e80326be354c55404a193b131b0b486e290'
readonly A26_ARM64_MANIFEST_SHA256='0cbc64571e00762b94fead8df8779ffe8563f08afad3160567130a3c0d99f1a4'
readonly A27_REPOSITORY='Insajin/autopus-adk' A28_TAG='v0.50.117' A28_VERSION='0.50.117'
# A27 published the same fifteen-asset set as A26, all measured from the
# immutable release 383500138. The two manifest digests come from inside the
# darwin archives, where the companion manifest travels.
readonly A27_RELEASE_ID='383500138'
readonly A27_COMMIT_SHA='fbe502c05f84d5eeb81b089b2344c47329ab4543'
readonly A27_TREE_SHA='95ba04d8499b00af86f6794e38a4da1b6d497a6d'
readonly A27_TAG_OBJECT_SHA='39101f97302267d052ed18b8aafa9ec23278c091'
readonly A27_CHECKSUMS_SHA256='8c57e0a9aa7cf86a7b2b4fe4f41e5982d0f04eb501125ec92fc45ae9715adac1'
readonly A27_AMD64_ARCHIVE_SHA256='476b420db5ede389c7bb59c4516b032c19f02530c7b36c77020aaa9f4b2b7587'
readonly A27_ARM64_ARCHIVE_SHA256='f3c7b2d148b370a7d200a3474ceb342f933c3adead08eca0044f63fbc8ed30bf'
readonly A27_LINUX_AMD64_ARCHIVE_SHA256='d3f3e2e582df1e32be530bdc6ab53302dcc3d4f37270eb02b5e66364fea1e334'
readonly A27_LINUX_ARM64_ARCHIVE_SHA256='e569ad405c5495321d0ee9557504c733a642e02b78d5d8fd88a027f59c148d65'
readonly A27_AMD64_MANIFEST_SHA256='c7c4ccac53798c59adeeffd724f8549a690bc8c60271fc57556a9e3f0cefdb54'
readonly A27_ARM64_MANIFEST_SHA256='1e9fa53467e81551a372edb9baecabd98d92d81ac4255b4492ccd6239c392dfa'
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
readonly A34_REPOSITORY='autopus-ai/autopus-adk' A35_TAG='v0.50.124' A35_VERSION='0.50.124'
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
else
  fail prior_release_identity_mismatch 'release is outside the frozen A0/A1/A2/A3/A4/A5/A6/A7/A8/A9/A10/A11/A12/A13/A14/A15/A16/A17/A18/A19/A20/A21/A22/A23/A24/A25/A26/A27/A28/A29/A30/A31/A32/A33/A34/A35 policy'
fi
