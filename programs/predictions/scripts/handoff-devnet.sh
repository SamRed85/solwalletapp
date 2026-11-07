#!/usr/bin/env bash
# Disabled on purpose. Do not move the upgrade authority to the council PDA.
#
# The council PDA can only sign through the predictions program, and a program cannot
# upgrade itself: the loader needs the program account writable and the runtime keeps the
# running program read-only (PrivilegeEscalation). Handing the authority to the council
# would freeze the program for good. The authority stays with the deployer key until it is
# moved to a real multisig (see "Upgrade authority" in .cursor/plans/master_plan.md).
echo "handoff-devnet.sh is disabled: the council cannot upgrade this program, so a handoff would lock it forever." >&2
exit 1
