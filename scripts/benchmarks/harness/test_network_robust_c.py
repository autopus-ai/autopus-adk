"""Pilot C contamination parsing never raises: one odd command line must not lose a trial."""
import unittest

import network_c


class NetworkRobustTests(unittest.TestCase):
    def test_a_trailing_backslash_does_not_raise(self):
        # The c03/full trial's command: shlex raises "No escaped character" on a trailing backslash.
        for line in ('echo "a\\', "printf 'x' \\", 'go test ./... \\', 'cat <<EOF\nok\\\nEOF'):
            self.assertIsNone(network_c.network_reason(line), line)

    def test_network_access_after_a_bad_quote_is_still_found(self):
        self.assertIsNotNone(network_c.network_reason('echo "unterminated ; curl https://example.com \\'))


if __name__ == '__main__':
    unittest.main()
