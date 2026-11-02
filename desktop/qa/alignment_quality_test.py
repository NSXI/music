import sys,unittest,copy
sys.path.insert(0,'alignment')
from quality import audit
class Quality(unittest.TestCase):
 def report(self,lines=None,text='First test',refined=True):
  return audit(lines or [{'text':'First test','words':[{'text':'First','start':.1,'end':.4,'probability':.9},{'text':' test','start':.6,'end':1.,'probability':.9}]}],text,2,'en',refined)
 def test_healthy(self):self.assertFalse(self.report()['needsReview'])
 def test_refinement_failure(self):self.assertTrue(self.report(refined=False)['needsReview'])
 def test_wrong_text(self):self.assertTrue(self.report(text='Missing text')['needsReview'])
 def test_overlap(self):self.assertTrue(self.report([{'text':'First test','words':[{'text':'First','start':.1,'end':.7,'probability':.9},{'text':' test','start':.5,'end':1,'probability':.9}]}])['needsReview'])
 def test_zero_duration(self):self.assertTrue(self.report([{'text':'First test','words':[{'text':'First test','start':.1,'end':.1,'probability':.9}]}])['needsReview'])
 def test_low_probability(self):self.assertTrue(self.report([{'text':'First test','words':[{'text':'First test','start':.1,'end':.9,'probability':.1}]}])['needsReview'])
 def test_arabic(self):self.assertFalse(self.report([{'text':'الموسيقى تجمعنا','words':[{'text':'الموسيقى','start':.1,'end':.5,'probability':.9},{'text':' تجمعنا','start':.6,'end':1,'probability':.9}]}],text='الموسيقى تجمعنا')['needsReview'])
 def test_letters_truthful(self):self.assertEqual(self.report()['characterTiming'],'visual-within-word')
unittest.main()
