import unittest


class QuxTest(unittest.TestCase):
    def test_qux(self):
        self.assertEqual("qux", "qux")


if __name__ == "__main__":
    unittest.main()
