---
description: INX-Coordinator is the extension tasked with coordinating the growth of the tangle.
image: /img/Banner/banner_hornet.png
keywords:
- IOTA Node
- Hornet Node
- INX
- Coordinator
- IOTA
- Shimmer
- Node Software
- Welcome
- explanation
---

# Welcome to INX-Coordinator

In the current implementation, the Tangle is secured by the [Coordinator](https://wiki.iota.org/learn/about-iota/coordinator) node. It issues special blocks: *milestones*. The rest of the network considers new blocks valid only if they reference a milestone.

## Setup

You need only one Coordinator to secure a network, so you want to install this extension only when you are going to run your own private network. See [Run a Private Tangle](https://wiki.iota.org/hornet/develop/how_tos/private_tangle) for details.


## Configuration

The coordinator connects to the local Hornet instance by default.

You can find all the configuration options in [Configuration](configuration.md).


## Source Code

The source code of the project is available on [GitHub](https://github.com/iotaledger/inx-coordinator).