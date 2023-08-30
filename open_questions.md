- do we check if the milestone message exists in the node at startup?
- node sync on startup?
- how is it enforced that tangle plugin is running at startup of coo?

		// wait until all background workers of the tangle plugin are started
		deps.Tangle.WaitForTangleProcessorStartup()



	// set the node as synced at startup, so the coo plugin can select tips
	deps.Tangle.SetUpdateSyncedAtStartup(true)

    - do the TODOs in the todo file

bootstrapping of networks.... do we really use genesis message ID here in all situations?
